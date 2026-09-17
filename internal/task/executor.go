// Package task executes warmup and webhook tasks: attempt loop with retry,
// execution/attempt recording, and final-result notifications.
package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-subscription-keeper/internal/crypto"
	"ai-subscription-keeper/internal/httpclient"
	"ai-subscription-keeper/internal/notify"
	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/registry"
	"ai-subscription-keeper/internal/store"
)

var ErrTaskBusy = errors.New("任务正在执行中")

type Executor struct {
	store    *store.Store
	enc      *crypto.Encryptor
	hc       *httpclient.Manager
	notifier *notify.Sender
	webhookClient *http.Client

	mu    sync.Mutex
	locks map[string]struct{}
}

func NewExecutor(st *store.Store, enc *crypto.Encryptor, hc *httpclient.Manager, notifier *notify.Sender) *Executor {
	return &Executor{
		store:         st,
		enc:           enc,
		hc:            hc,
		notifier:      notifier,
		webhookClient: &http.Client{Timeout: 60 * time.Second},
		locks:         map[string]struct{}{},
	}
}

func (e *Executor) acquire(taskID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, busy := e.locks[taskID]; busy {
		return false
	}
	e.locks[taskID] = struct{}{}
	return true
}

func (e *Executor) release(taskID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.locks, taskID)
}

// Running reports whether the task currently holds an execution slot.
func (e *Executor) Running(taskID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, busy := e.locks[taskID]
	return busy
}

// RunAsync executes the task in a background goroutine (cron triggers and
// 立即执行). Returns ErrTaskBusy when an instance is already running.
func (e *Executor) RunAsync(t *store.Task) error {
	if !e.acquire(t.ID) {
		return ErrTaskBusy
	}
	go func() {
		defer e.release(t.ID)
		if err := e.run(t); err != nil {
			fmt.Printf("[task] %s 执行失败: %v\n", t.Name, err)
		}
	}()
	return nil
}

func (e *Executor) run(t *store.Task) error {
	exec := &store.Execution{TaskID: t.ID}
	if err := e.store.CreateExecution(exec); err != nil {
		return err
	}
	var status, result, errMsg string
	var attempts int
	if t.Type == store.TaskTypeWebhook {
		status, result, errMsg, attempts = e.runWebhook(t, exec.ID)
	} else {
		status, result, errMsg, attempts = e.runWarmup(t, exec.ID)
	}
	if err := e.store.FinishExecution(exec.ID, status, result, errMsg, attempts, time.Now().UTC()); err != nil {
		return err
	}
	e.notifyResult(t, exec.ID, status, result, errMsg, attempts)
	return nil
}

// ------------------------------------------------------------------ warmup --

type dimensionBrief struct {
	Label       string  `json:"label"`
	UsedPercent float64 `json:"used_percent"`
}

type warmupAttemptResult struct {
	Attempt          int              `json:"attempt"`
	ModelOK          bool             `json:"model_ok"`
	ModelError       string           `json:"model_error,omitempty"`
	ModelDurationMS  int64            `json:"model_duration_ms,omitempty"`
	QuotaOK          bool             `json:"quota_ok"`
	QuotaUnsupported bool             `json:"quota_unsupported,omitempty"`
	QuotaError       string           `json:"quota_error,omitempty"`
	ResetAt          string           `json:"reset_at,omitempty"`
	Dimensions       []dimensionBrief `json:"dimensions,omitempty"`
	ContentPreview   string           `json:"content_preview,omitempty"`
}

// runWarmup runs one full warmup cycle: streaming model request → quota query
// → final judgement, retrying per task config. Each attempt is persisted.
func (e *Executor) runWarmup(t *store.Task, executionID string) (status, result, errMsg string, attempts int) {
	loc := taskLocation(t.Timezone)

	if t.ModelServiceID == "" {
		return store.ExecStatusFailed, "", "未配置模型服务", 0
	}
	ms, err := e.store.GetModelService(t.ModelServiceID)
	if err != nil {
		return store.ExecStatusFailed, "", "模型服务不存在: " + err.Error(), 0
	}
	cred, err := e.decryptCredential(ms.Credential)
	if err != nil {
		return store.ExecStatusFailed, "", "解密凭证失败: " + err.Error(), 0
	}
	pv, err := registry.Get(ms.ProviderType, cred, e.hc)
	if err != nil {
		return store.ExecStatusFailed, "", err.Error(), 0
	}

	maxAttempts := t.RetryCount + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr string
	var success *warmupAttemptResult
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attempts = attempt
		attemptID, aerr := e.store.CreateExecutionAttempt(&store.ExecutionAttempt{
			ExecutionID: executionID, Attempt: attempt,
		})
		if aerr != nil {
			return store.ExecStatusFailed, "", "记录执行尝试失败: " + aerr.Error(), attempts
		}

		ar := &warmupAttemptResult{Attempt: attempt}

		// 1. Streaming model request.
		sr, merr := pv.ChatStream(context.Background(), provider.ModelRequest{Model: t.Model, Prompt: t.Prompt}, nil)
		ar.ModelOK = merr == nil
		if merr != nil {
			ar.ModelError = merr.Error()
			lastErr = "模型请求: " + merr.Error()
		} else {
			ar.ModelDurationMS = sr.DurationMS
			ar.ContentPreview = preview(sr.Content, 200)
		}

		// 2. Quota query (skipped when the vendor has no quota API).
		if merr == nil && pv.SupportsQuota() {
			qi, _, qerr := pv.GetQuota(context.Background())
			if qerr != nil {
				ar.QuotaError = qerr.Error()
				lastErr = "额度查询: " + qerr.Error()
			} else {
				ar.QuotaOK = true
				for _, d := range qi.Dimensions {
					b := dimensionBrief{Label: d.DisplayName}
					if d.UsedPercentage != nil {
						b.UsedPercent = *d.UsedPercentage
					}
					ar.Dimensions = append(ar.Dimensions, b)
					if d.ResetAt != nil {
						ar.ResetAt = d.ResetAt.In(loc).Format("01-02 15:04")
					}
				}
			}
		} else {
			ar.QuotaUnsupported = true
			ar.QuotaOK = true
		}

		ok := ar.ModelOK && ar.QuotaOK
		_ = e.store.FinishExecutionAttempt(attemptID,
			statusWord(ar.ModelOK), statusWord(ar.QuotaOK), 0,
			mustJSON(ar), attemptError(ar))

		if ok {
			success = ar
			break
		}
		if attempt < maxAttempts && t.RetryIntervalMinutes > 0 {
			time.Sleep(time.Duration(t.RetryIntervalMinutes) * time.Minute)
		}
	}

	if success != nil {
		return store.ExecStatusSuccess, mustJSON(map[string]any{
			"model_service":   ms.Name,
			"model":           t.Model,
			"attempts":        attempts,
			"dimensions":      success.Dimensions,
			"reset_at":        success.ResetAt,
			"content_preview": success.ContentPreview,
		}), "", attempts
	}
	return store.ExecStatusFailed, "", lastErr, attempts
}

// ----------------------------------------------------------------- webhook --

type webhookConfig struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type webhookAttemptResult struct {
	Attempt    int    `json:"attempt"`
	HTTPStatus int    `json:"http_status"`
	DurationMS int64  `json:"duration_ms"`
	Response   string `json:"response,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (e *Executor) runWebhook(t *store.Task, executionID string) (status, result, errMsg string, attempts int) {
	var cfg webhookConfig
	if err := json.Unmarshal([]byte(t.WebhookConfig), &cfg); err != nil {
		return store.ExecStatusFailed, "", "Webhook 配置无效: " + err.Error(), 0
	}
	if cfg.URL == "" {
		return store.ExecStatusFailed, "", "Webhook URL 未配置", 0
	}
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodPost
	}
	maxAttempts := t.RetryCount + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var last *webhookAttemptResult
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attempts = attempt
		attemptID, aerr := e.store.CreateExecutionAttempt(&store.ExecutionAttempt{
			ExecutionID: executionID, Attempt: attempt,
		})
		if aerr != nil {
			return store.ExecStatusFailed, "", "记录执行尝试失败: " + aerr.Error(), attempts
		}

		ar := &webhookAttemptResult{Attempt: attempt}
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		body := renderTemplates(cfg.Body, t.Name, executionID, attempt, status)
		req, err := http.NewRequestWithContext(ctx, method,
			renderTemplates(cfg.URL, t.Name, executionID, attempt, status),
			strings.NewReader(body))
		if err != nil {
			ar.Error = err.Error()
		} else {
			if body != "" && req.Header.Get("Content-Type") == "" {
				req.Header.Set("Content-Type", "application/json")
			}
			for k, v := range cfg.Headers {
				req.Header.Set(k, renderTemplates(v, t.Name, executionID, attempt, status))
			}
			resp, err := e.webhookClient.Do(req)
			if err != nil {
				ar.Error = err.Error()
			} else {
				ar.HTTPStatus = resp.StatusCode
				raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
				resp.Body.Close()
				ar.Response = preview(string(raw), 2000)
				if resp.StatusCode/100 != 2 {
					ar.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
				}
			}
		}
		cancel()
		ar.DurationMS = time.Since(start).Milliseconds()
		last = ar

		_ = e.store.FinishExecutionAttempt(attemptID, "",
			statusWord(ar.Error == ""), ar.HTTPStatus, mustJSON(ar), ar.Error)

		if ar.Error == "" {
			break
		}
		if attempt < maxAttempts && t.RetryIntervalMinutes > 0 {
			time.Sleep(time.Duration(t.RetryIntervalMinutes) * time.Minute)
		}
	}
	if last != nil && last.Error == "" {
		return store.ExecStatusSuccess, mustJSON(last), "", attempts
	}
	if last != nil {
		return store.ExecStatusFailed, mustJSON(last), last.Error, attempts
	}
	return store.ExecStatusFailed, "", "未执行任何请求", 0
}

func renderTemplates(s, taskName, executionID string, attempt int, status string) string {
	now := time.Now()
	r := strings.NewReplacer(
		"{{date}}", now.Format("2006-01-02"),
		"{{time}}", now.Format("15:04:05"),
		"{{timestamp}}", strconv.FormatInt(now.Unix(), 10),
		"{{task.name}}", taskName,
		"{{execution.id}}", executionID,
		"{{attempt}}", strconv.Itoa(attempt),
		"{{status}}", status,
	)
	return r.Replace(s)
}

// ------------------------------------------------------------ notification --

func (e *Executor) notifyResult(t *store.Task, executionID, status, result, errMsg string, attempts int) {
	if len(t.NotificationChannelIDs) == 0 {
		return
	}
	exec, err := e.store.GetExecution(executionID)
	if err != nil {
		return
	}
	loc := taskLocation(t.Timezone)
	title, content := e.renderNotification(t, exec, status, errMsg, attempts, result, loc)
	event := "task_failure"
	if status == store.ExecStatusSuccess {
		event = "task_success"
	}
	msg := notify.Message{
		Event:   event,
		Title:   title,
		Content: content,
		Task:    t.Name,
		At:      time.Now().In(loc).Format(time.RFC3339),
	}
	// 逐个渠道发送；单渠道失败不影响其余渠道。
	for _, cid := range t.NotificationChannelIDs {
		ch, err := e.store.GetNotificationChannel(cid)
		if err != nil || !ch.Enabled {
			continue
		}
		cfgJSON, err := e.enc.Decrypt(ch.Config)
		if err != nil {
			fmt.Printf("[notify] 解密通知配置失败 (%s): %v\n", ch.Name, err)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := e.notifier.Send(ctx, ch.Type, cfgJSON, msg); err != nil {
			fmt.Printf("[notify] 发送失败 (%s → %s): %v\n", t.Name, ch.Name, err)
		}
		cancel()
	}
}

func (e *Executor) renderNotification(t *store.Task, exec *store.Execution, status, errMsg string, attempts int, result string, loc *time.Location) (string, string) {
	success := status == store.ExecStatusSuccess
	outcome := "失败"
	icon := "❌"
	if success {
		outcome = "成功"
		icon = "✅"
	}
	var b strings.Builder
	if t.Type == store.TaskTypeWebhook {
		var ar webhookAttemptResult
		_ = json.Unmarshal([]byte(result), &ar)
		b.WriteString("任务：" + t.Name + "\n")
		b.WriteString("类型：Webhook\n\n")
		b.WriteString("开始：" + exec.StartedAt.In(loc).Format("15:04:05") + "\n")
		if exec.FinishedAt != nil {
			b.WriteString("结束：" + exec.FinishedAt.In(loc).Format("15:04:05") + "\n")
		}
		b.WriteString("重试：" + strconv.Itoa(attempts-1) + "\n")
		if ar.HTTPStatus > 0 {
			b.WriteString(fmt.Sprintf("HTTP 状态：%d\n", ar.HTTPStatus))
		}
		if errMsg != "" {
			b.WriteString("\n错误：\n" + preview(errMsg, 500) + "\n")
		}
		b.WriteString("\n状态：" + outcome)
		return icon + " Webhook 任务" + outcome, b.String()
	}

	b.WriteString("任务：" + t.Name + "\n")
	if ms, err := e.store.GetModelService(t.ModelServiceID); err == nil {
		b.WriteString("模型服务：" + ms.Name + "\n")
	}
	b.WriteString("模型：" + t.Model + "\n\n")
	b.WriteString("开始：" + exec.StartedAt.In(loc).Format("15:04:05") + "\n")
	if exec.FinishedAt != nil {
		b.WriteString("结束：" + exec.FinishedAt.In(loc).Format("15:04:05") + "\n")
	}
	b.WriteString("重试：" + strconv.Itoa(attempts-1) + "\n\n")
	if success {
		var wr struct {
			Dimensions []dimensionBrief `json:"dimensions"`
			ResetAt    string          `json:"reset_at"`
		}
		_ = json.Unmarshal([]byte(result), &wr)
		for _, d := range wr.Dimensions {
			b.WriteString(fmt.Sprintf("%s：剩余 %.0f%%\n", d.Label, 100-d.UsedPercent))
		}
		if wr.ResetAt != "" {
			b.WriteString("重置时间：" + wr.ResetAt + "\n")
		}
	} else {
		b.WriteString("尝试次数：" + strconv.Itoa(attempts) + "\n")
		b.WriteString("\n错误：\n" + preview(errMsg, 500) + "\n")
	}
	b.WriteString("\n状态：" + outcome)
	return icon + " 模型预热" + outcome, b.String()
}

// ---------------------------------------------------------------- helpers --

func (e *Executor) decryptCredential(encBlob string) (*provider.Credential, error) {
	if encBlob == "" {
		return &provider.Credential{}, nil
	}
	plain, err := e.enc.Decrypt(encBlob)
	if err != nil {
		return nil, err
	}
	var cred provider.Credential
	if err := json.Unmarshal([]byte(plain), &cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

func statusWord(ok bool) string {
	if ok {
		return "success"
	}
	return "failed"
}

func attemptError(ar *warmupAttemptResult) string {
	if ar.ModelError != "" {
		return ar.ModelError
	}
	return ar.QuotaError
}

func taskLocation(tz string) *time.Location {
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Local
	}
	return loc
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func preview(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
