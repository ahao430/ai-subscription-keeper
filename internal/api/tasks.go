package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ai-subscription-keeper/internal/app"
	"ai-subscription-keeper/internal/store"
)

type taskRequest struct {
	Name                  string          `json:"name"`
	Type                  string          `json:"type"`
	ModelServiceID        string          `json:"model_service_id"`
	Model                 string          `json:"model"`
	Prompt                string          `json:"prompt"`
	Cron                  string          `json:"cron"`
	Timezone              string          `json:"timezone"`
	WebhookConfig         json.RawMessage `json:"webhook_config"`
	RetryCount            int             `json:"retry_count"`
	RetryIntervalMinutes  int             `json:"retry_interval_min"`
	NotificationChannelIDs []string       `json:"notification_channel_ids"`
	Enabled               *bool           `json:"enabled"`
}

func (req *taskRequest) apply(t *store.Task) {
	if strings.TrimSpace(req.Name) != "" {
		t.Name = strings.TrimSpace(req.Name)
	}
	t.Type = req.Type
	t.ModelServiceID = req.ModelServiceID
	t.Model = req.Model
	if req.Prompt != "" {
		t.Prompt = req.Prompt
	}
	t.Cron = strings.TrimSpace(req.Cron)
	if req.Timezone != "" {
		t.Timezone = req.Timezone
	}
	if len(req.WebhookConfig) > 0 {
		t.WebhookConfig = string(req.WebhookConfig)
	}
	t.RetryCount = req.RetryCount
	t.RetryIntervalMinutes = req.RetryIntervalMinutes
	t.NotificationChannelIDs = req.NotificationChannelIDs
	if req.Enabled != nil {
		t.Enabled = *req.Enabled
	}
}

func validateTask(a *app.App, t *store.Task) error {
	if strings.TrimSpace(t.Name) == "" {
		return errStr("任务名称不能为空")
	}
	if t.Type != store.TaskTypeWarmup && t.Type != store.TaskTypeWebhook {
		return errStr("任务类型必须是 warmup 或 webhook")
	}
	if t.Type == store.TaskTypeWarmup {
		if t.ModelServiceID == "" {
			return errStr("请选择模型服务")
		}
		if _, err := a.Store.GetModelService(t.ModelServiceID); err != nil {
			return errStr("模型服务不存在")
		}
		if t.Model == "" {
			return errStr("请选择预热模型")
		}
	}
	if t.Type == store.TaskTypeWebhook && t.WebhookConfig == "" {
		t.WebhookConfig = "{}"
	}
	for _, id := range t.NotificationChannelIDs {
		if _, err := a.Store.GetNotificationChannel(id); err != nil {
			return errStr("通知渠道不存在: " + id)
		}
	}
	return nil
}

type errStr string

func (e errStr) Error() string { return string(e) }

func handleListTasks(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tasks, err := a.Store.ListTasks()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		// Decorate with service/channel names, next fire time and the latest
		// execution result for dashboard display.
		type taskRow struct {
			*store.Task
			ModelServiceName string            `json:"model_service_name"`
			ChannelNames     []string          `json:"notification_channel_names"`
			Running          bool              `json:"running"`
			NextRunAt        *time.Time        `json:"next_run_at,omitempty"`
			LastExecution    *store.Execution  `json:"last_execution,omitempty"`
		}
		out := make([]*taskRow, 0, len(tasks))
		for _, t := range tasks {
			row := &taskRow{Task: t}
			if t.ModelServiceID != "" {
				if ms, err := a.Store.GetModelService(t.ModelServiceID); err == nil {
					row.ModelServiceName = ms.Name
				}
			}
			for _, cid := range t.NotificationChannelIDs {
				if ch, err := a.Store.GetNotificationChannel(cid); err == nil {
					row.ChannelNames = append(row.ChannelNames, ch.Name)
				}
			}
			if row.ChannelNames == nil {
				row.ChannelNames = []string{}
			}
			row.Running = a.Exec.Running(t.ID)
			if next, ok := a.Sched.NextRun(t.ID); ok {
				n := next
				row.NextRunAt = &n
			}
			row.LastExecution, _ = a.Store.LatestExecution(t.ID)
			out = append(out, row)
		}
		writeJSON(w, http.StatusOK, map[string]any{"tasks": out})
	}
}

func handleCreateTask(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req taskRequest
		if err := decodeBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		t := &store.Task{}
		req.apply(t)
		if t.Timezone == "" {
			t.Timezone = "Asia/Shanghai"
		}
		if req.Enabled == nil {
			t.Enabled = true
		}
		if err := validateTask(a, t); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := a.Store.CreateTask(t); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if t.Enabled {
			if err := a.Sched.Schedule(t); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, t)
	}
}

func handleUpdateTask(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := a.Store.GetTask(r.PathValue("id"))
		if err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		var req taskRequest
		if err := decodeBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		req.apply(t)
		if err := validateTask(a, t); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := a.Store.UpdateTask(t); err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		a.Sched.Unschedule(t.ID)
		if t.Enabled {
			if err := a.Sched.Schedule(t); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, t)
	}
}

func handleDeleteTask(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := a.Store.DeleteTask(id); err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		a.Sched.Unschedule(id)
		writeJSON(w, http.StatusOK, map[string]string{"ok": "deleted"})
	}
}

// handleRunTask triggers 立即执行. The execution runs in the background; the
// created execution id is returned for polling.
func handleRunTask(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := a.Store.GetTask(r.PathValue("id"))
		if err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		if err := a.Exec.RunAsync(t); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
	}
}

func handleTaskExecutions(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := a.Store.GetTask(r.PathValue("id"))
		if err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		_ = t
		executions, err := a.Store.ListExecutions(t.ID, 50)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if executions == nil {
			executions = []*store.Execution{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"executions": executions})
	}
}

func handleGetExecution(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		exec, err := a.Store.GetExecution(r.PathValue("id"))
		if err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		attempts, err := a.Store.ListExecutionAttempts(exec.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if attempts == nil {
			attempts = []*store.ExecutionAttempt{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"execution": exec, "attempts": attempts})
	}
}

func handleDeleteTaskExecutions(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := r.PathValue("id")
		if err := a.Store.DeleteExecutions(taskID); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"ok": "deleted"})
	}
}

func handleLogCleanup(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			OlderThanDays int `json:"older_than_days"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if body.OlderThanDays <= 0 {
			body.OlderThanDays = 30
		}
		cutoff := time.Now().UTC().AddDate(0, 0, -body.OlderThanDays)
		execN, err := a.Store.PruneExecutions(cutoff)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		testN, err := a.Store.PruneTestLogs(cutoff)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"deleted_executions": execN,
			"deleted_test_logs":  testN,
		})
	}
}
