package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"ai-subscription-keeper/internal/app"
	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/registry"
	"ai-subscription-keeper/internal/store"
)

// serviceRequest is the create/update payload. Empty secret fields on update
// mean "keep the stored value".
type serviceRequest struct {
	Name               string            `json:"name"`
	ProviderType       string            `json:"provider_type"`
	Credential         map[string]string `json:"credential"`
	DefaultWarmupModel string            `json:"default_warmup_model"`
	DefaultTestModel   string            `json:"default_test_model"`
	DefaultTestPrompt  string            `json:"default_test_prompt"`
	QuotaLabels        map[string]string `json:"quota_labels"`
	BillingType        string            `json:"billing_type"`
	Enabled            *bool             `json:"enabled"`
	SortOrder          int               `json:"sort_order"`
}

// serviceResponse is what clients see — credentials come back masked.
type serviceResponse struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	ProviderType      string            `json:"provider_type"`
	Credential        map[string]string `json:"credential"`
	Models            []string          `json:"models"`
	DefaultWarmupModel string           `json:"default_warmup_model"`
	DefaultTestModel  string            `json:"default_test_model"`
	DefaultTestPrompt string            `json:"default_test_prompt"`
	QuotaLabels       map[string]string `json:"quota_labels"`
	BillingType       string            `json:"billing_type"`
	SortOrder         int               `json:"sort_order"`
	Enabled           bool              `json:"enabled"`
	LastQuotaAt       *time.Time        `json:"last_quota_at,omitempty"`
	LastQuotaError    string            `json:"last_quota_error,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

var secretCredentialKeys = map[string]bool{"api_key": true, "system_token": true}

func toServiceResponse(ms *store.ModelService) *serviceResponse {
	resp := &serviceResponse{
		ID:                 ms.ID,
		Name:               ms.Name,
		ProviderType:       ms.ProviderType,
		DefaultWarmupModel: ms.DefaultWarmupModel,
		DefaultTestModel:   ms.DefaultTestModel,
		DefaultTestPrompt:  ms.DefaultTestPrompt,
		QuotaLabels:        map[string]string{},
		BillingType:        ms.BillingType,
		SortOrder:          ms.SortOrder,
		Enabled:            ms.Enabled,
		LastQuotaAt:        ms.LastQuotaAt,
		LastQuotaError:     ms.LastQuotaError,
		CreatedAt:          ms.CreatedAt,
		UpdatedAt:          ms.UpdatedAt,
	}
	if resp.BillingType == "" {
		resp.BillingType = "subscription"
	}
	_ = json.Unmarshal([]byte(ms.Models), &resp.Models)
	if resp.Models == nil {
		resp.Models = []string{}
	}
	_ = json.Unmarshal([]byte(orDefaultJSON(ms.QuotaLabels)), &resp.QuotaLabels)
	resp.Credential = map[string]string{}
	// Credential stays encrypted in the store; we never echo secrets back.
	return resp
}

func orDefaultJSON(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}

// mergeCredential builds the credential JSON from the form map, preserving
// stored secrets for blank secret fields.
func mergeCredential(a *app.App, ms *store.ModelService, form map[string]string) (string, error) {
	cred := &provider.Credential{}
	if form != nil {
		raw, _ := json.Marshal(form)
		_ = json.Unmarshal(raw, cred)
	}
	if ms != nil && ms.Credential != "" {
		var old provider.Credential
		plain, err := a.Enc.Decrypt(ms.Credential)
		if err == nil {
			_ = json.Unmarshal([]byte(plain), &old)
		}
		if cred.APIKey == "" {
			cred.APIKey = old.APIKey
		}
		if cred.SystemToken == "" {
			cred.SystemToken = old.SystemToken
		}
	}
	return a.EncryptCredential(cred)
}

func handleListServices(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		services, err := a.Store.ListModelServices()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		out := make([]*serviceResponse, 0, len(services))
		for _, ms := range services {
			out = append(out, toServiceResponse(ms))
		}
		writeJSON(w, http.StatusOK, map[string]any{"services": out})
	}
}

func handleCreateService(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req serviceRequest
		if err := decodeBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("名称不能为空"))
			return
		}
		if _, ok := registry.TypeByCode(req.ProviderType); !ok {
			writeError(w, http.StatusBadRequest, fmt.Errorf("未知供应商: %s", req.ProviderType))
			return
		}
		credJSON, err := mergeCredential(a, nil, req.Credential)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		ms := &store.ModelService{
			Name: strings.TrimSpace(req.Name),
			ProviderType: req.ProviderType,
			Credential: credJSON,
			DefaultWarmupModel: req.DefaultWarmupModel,
			DefaultTestModel: req.DefaultTestModel,
			DefaultTestPrompt: req.DefaultTestPrompt,
			QuotaLabels: marshalLabels(req.QuotaLabels),
			BillingType: req.BillingType,
			SortOrder: req.SortOrder,
			Enabled: enabled,
		}
		if err := a.Store.CreateModelService(ms); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		// Populate the model list right away when possible.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			fetchAndSaveModels(ctx, a, ms)
		}()
		writeJSON(w, http.StatusOK, toServiceResponse(ms))
	}
}

func marshalLabels(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func handleGetService(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ms, err := a.Store.GetModelService(r.PathValue("id"))
		if err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, toServiceResponse(ms))
	}
}

func handleUpdateService(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ms, err := a.Store.GetModelService(r.PathValue("id"))
		if err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		var req serviceRequest
		if err := decodeBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.Name) != "" {
			ms.Name = strings.TrimSpace(req.Name)
		}
		if req.ProviderType != "" {
			if _, ok := registry.TypeByCode(req.ProviderType); !ok {
				writeError(w, http.StatusBadRequest, fmt.Errorf("未知供应商: %s", req.ProviderType))
				return
			}
			ms.ProviderType = req.ProviderType
		}
		credJSON, err := mergeCredential(a, ms, req.Credential)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		ms.Credential = credJSON
		ms.DefaultWarmupModel = req.DefaultWarmupModel
		ms.DefaultTestModel = req.DefaultTestModel
		if req.DefaultTestPrompt != "" {
			ms.DefaultTestPrompt = req.DefaultTestPrompt
		}
		ms.QuotaLabels = marshalLabels(req.QuotaLabels)
		if req.BillingType != "" {
			ms.BillingType = req.BillingType
		}
		ms.SortOrder = req.SortOrder
		if req.Enabled != nil {
			ms.Enabled = *req.Enabled
		}
		if err := a.Store.UpdateModelService(ms); err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, toServiceResponse(ms))
	}
}

func handleDeleteService(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.DeleteModelService(r.PathValue("id")); err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"ok": "deleted"})
	}
}

func handleRefreshService(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ms, err := a.Store.GetModelService(r.PathValue("id"))
		if err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		a.RefreshQuota(ctx, ms)
		updated, err := a.Store.GetModelService(ms.ID)
		if err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, buildDashboardCard(updated))
	}
}

// fetchAndSaveModels queries the provider model list and persists it.
func fetchAndSaveModels(ctx context.Context, a *app.App, ms *store.ModelService) ([]string, error) {
	pv, err := a.ProviderFor(ms)
	if err != nil {
		return nil, err
	}
	models, err := pv.GetModels(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.ID)
	}
	sort.Strings(ids)
	b, _ := json.Marshal(ids)
	if err := a.Store.UpdateModelServiceModels(ms.ID, string(b)); err != nil {
		return nil, err
	}
	return ids, nil
}

func handleRefreshModels(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ms, err := a.Store.GetModelService(r.PathValue("id"))
		if err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		ids, err := fetchAndSaveModels(ctx, a, ms)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": ids})
	}
}

// handleAdhocModels backs the "测试连接 / 获取模型" buttons in the service
// editor before anything is saved.
func handleAdhocModels(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.PathValue("code")
		if _, ok := registry.TypeByCode(code); !ok {
			writeError(w, http.StatusBadRequest, fmt.Errorf("未知供应商: %s", code))
			return
		}
		var body struct {
			Credential map[string]string `json:"credential"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		cred := &provider.Credential{}
		if body.Credential != nil {
			raw, _ := json.Marshal(body.Credential)
			_ = json.Unmarshal(raw, cred)
		}
		pv, err := registry.Get(code, cred, a.HC)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		models, err := pv.GetModels(ctx)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		ids := make([]string, 0, len(models))
		for _, m := range models {
			ids = append(ids, m.ID)
		}
		sort.Strings(ids)
		writeJSON(w, http.StatusOK, map[string]any{"models": ids})
	}
}

func handleReorderServices(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IDs []string `json:"ids"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := a.Store.ReorderModelServices(body.IDs); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleRawQuota(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ms, err := a.Store.GetModelService(r.PathValue("id"))
		if err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		var payload app.QuotaPayload
		if ms.LastQuota != "" {
			_ = json.Unmarshal([]byte(ms.LastQuota), &payload)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"service_id":        ms.ID,
			"service_name":      ms.Name,
			"unsupported":       payload.Unsupported,
			"raw":               payload.Raw,
			"last_quota_at":     ms.LastQuotaAt,
			"last_quota_error":  ms.LastQuotaError,
		})
	}
}

// handleTestService streams a model test over SSE. Events:
//
//	data: {"type":"meta","model":...,"prompt":...}
//	data: {"type":"delta","content":...}
//	data: {"type":"done","result":{...}} | {"type":"error","error":"..."}
func handleTestService(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ms, err := a.Store.GetModelService(r.PathValue("id"))
		if err != nil {
			writeError(w, errorStatus(err), err)
			return
		}
		var body struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
		}
		if r.Body != nil {
			_ = decodeBody(r, &body)
		}
		model := body.Model
		if model == "" {
			model = ms.DefaultTestModel
		}
		prompt := body.Prompt
		if prompt == "" {
			prompt = ms.DefaultTestPrompt
		}
		if prompt == "" {
			prompt = "hi"
		}
		if model == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("未配置测试模型"))
			return
		}
		pv, err := a.ProviderFor(ms)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		// Persist test log.
		log := &store.TestLog{
			ServiceID: ms.ID,
			Model:     model,
			Prompt:    prompt,
			Status:    store.TestLogStatusRunning,
		}
		_ = a.Store.CreateTestLog(log)
		logID := log.ID

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("流式响应不可用"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Ael-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		send := func(v any) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}

		send(map[string]any{"type": "meta", "model": model, "prompt": prompt})

		result, err := pv.ChatStream(r.Context(), provider.ModelRequest{Model: model, Prompt: prompt}, func(delta string) {
			send(map[string]any{"type": "delta", "content": delta})
		})
		if err != nil {
			msg := err.Error()
			resultJSON := ""
			if result != nil {
				rb, _ := json.Marshal(result)
				resultJSON = string(rb)
				send(map[string]any{"type": "done", "result": result, "error": msg})
				_ = a.Store.FinishTestLog(logID, store.TestLogStatusFailed, resultJSON, msg)
				return
			}
			send(map[string]any{"type": "error", "error": msg})
			_ = a.Store.FinishTestLog(logID, store.TestLogStatusFailed, "", msg)
			return
		}
		rb, _ := json.Marshal(result)
		send(map[string]any{"type": "done", "result": result})
		_ = a.Store.FinishTestLog(logID, store.TestLogStatusSuccess, string(rb), "")
	}
}

// handleListTestLogs returns test logs for a model service.
func handleListTestLogs(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logs, err := a.Store.ListTestLogs(r.PathValue("id"), 50)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
	}
}

// handleDeleteTestLogs clears all test logs for a model service.
func handleDeleteTestLogs(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.DeleteTestLogs(r.PathValue("id")); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"ok": "deleted"})
	}
}
