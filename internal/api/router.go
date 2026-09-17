package api

import (
	"io/fs"
	"net/http"
	"strings"

	"ai-subscription-keeper/internal/app"
	"ai-subscription-keeper/internal/version"
	"ai-subscription-keeper/internal/webui"
)

// NewRouter mounts the REST API and the embedded SPA.
func NewRouter(a *app.App) http.Handler {
	mux := http.NewServeMux()

	// Version.
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": version.Version})
	})
	mux.HandleFunc("GET /api/version/update-check", handleCheckUpdate(a))
	mux.HandleFunc("POST /api/version/upgrade", handleUpgrade(a))

	// Provider types.
	mux.HandleFunc("GET /api/provider-types", handleProviderTypes)

	// Dashboard.
	mux.HandleFunc("GET /api/dashboard", handleDashboard(a))
	mux.HandleFunc("POST /api/dashboard/refresh", handleDashboardRefresh(a))

	// Model services.
	mux.HandleFunc("GET /api/model-services", handleListServices(a))
	mux.HandleFunc("POST /api/model-services", handleCreateService(a))
	mux.HandleFunc("POST /api/providers/{code}/models", handleAdhocModels(a))
	mux.HandleFunc("GET /api/model-services/{id}", handleGetService(a))
	mux.HandleFunc("PUT /api/model-services/{id}", handleUpdateService(a))
	mux.HandleFunc("DELETE /api/model-services/{id}", handleDeleteService(a))
	mux.HandleFunc("POST /api/model-services/{id}/refresh", handleRefreshService(a))
	mux.HandleFunc("POST /api/model-services/{id}/models/refresh", handleRefreshModels(a))
	mux.HandleFunc("POST /api/model-services/{id}/test", handleTestService(a))
	mux.HandleFunc("GET /api/model-services/{id}/test-logs", handleListTestLogs(a))
	mux.HandleFunc("DELETE /api/model-services/{id}/test-logs", handleDeleteTestLogs(a))
	mux.HandleFunc("GET /api/model-services/{id}/raw-quota", handleRawQuota(a))
	mux.HandleFunc("PUT /api/model-services/order", handleReorderServices(a))

	// Tasks & executions.
	mux.HandleFunc("GET /api/tasks", handleListTasks(a))
	mux.HandleFunc("POST /api/tasks", handleCreateTask(a))
	mux.HandleFunc("PUT /api/tasks/{id}", handleUpdateTask(a))
	mux.HandleFunc("DELETE /api/tasks/{id}", handleDeleteTask(a))
	mux.HandleFunc("POST /api/tasks/{id}/run", handleRunTask(a))
	mux.HandleFunc("GET /api/tasks/{id}/executions", handleTaskExecutions(a))
	mux.HandleFunc("DELETE /api/tasks/{id}/executions", handleDeleteTaskExecutions(a))
	mux.HandleFunc("GET /api/executions/{id}", handleGetExecution(a))

	// Log cleanup.
	mux.HandleFunc("POST /api/logs/cleanup", handleLogCleanup(a))

	// Notifications.
	mux.HandleFunc("GET /api/notifications", handleListChannels(a))
	mux.HandleFunc("POST /api/notifications", handleCreateChannel(a))
	mux.HandleFunc("PUT /api/notifications/{id}", handleUpdateChannel(a))
	mux.HandleFunc("DELETE /api/notifications/{id}", handleDeleteChannel(a))
	mux.HandleFunc("POST /api/notifications/{id}/test", handleTestChannel(a))

	// Settings.
	mux.HandleFunc("GET /api/settings/proxy", handleGetProxy(a))
	mux.HandleFunc("PUT /api/settings/proxy", handlePutProxy(a))
	mux.HandleFunc("POST /api/settings/proxy/test", handleTestProxy(a))
	mux.HandleFunc("GET /api/settings/export", handleExportConfig(a))
	mux.HandleFunc("POST /api/settings/import", handleImportConfig(a))
	mux.HandleFunc("GET /api/settings/webdav", handleGetWebdavConfig(a))
	mux.HandleFunc("PUT /api/settings/webdav", handlePutWebdavConfig(a))
	mux.HandleFunc("POST /api/settings/webdav/test", handleTestWebdav(a))
	mux.HandleFunc("POST /api/settings/webdav/sync", handleWebdavSync(a))
	mux.HandleFunc("POST /api/settings/webdav/restore", handleWebdavRestore(a))

	// Embedded frontend (SPA with history fallback).
	dist, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(dist))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" && !strings.HasPrefix(path, "api/") {
			if f, err := dist.Open(path); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// History fallback → index.html.
		index, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			http.Error(w, "前端未构建：请先构建 web/ 并重新编译", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})

	return mux
}
