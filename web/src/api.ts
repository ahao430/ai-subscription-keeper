import type {
  DashboardService,
  Execution,
  ExecutionAttempt,
  ModelService,
  NotificationChannel,
  ProviderTypeInfo,
  ProxyConfig,
  StreamResult,
  Task,
  TestLog,
} from './types';

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
  const text = await res.text();
  let data: unknown = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    /* non-JSON body */
  }
  if (!res.ok) {
    const msg =
      data && typeof data === 'object' && 'error' in data
        ? String((data as { error: unknown }).error)
        : `HTTP ${res.status}`;
    throw new ApiError(res.status, msg);
  }
  return data as T;
}

export const api = {
  // Version
  version: () => request<{ version: string }>('/api/version'),
  checkUpdate: () =>
    request<{
      current_version: string;
      latest_version: string;
      update_available: boolean;
      release_url: string;
      published_at: string;
      notes?: string;
    }>('/api/version/update-check'),
  upgradeVersion: (tag?: string) =>
    request<{ ok: boolean; old_version: string; new_version: string; restarting: boolean }>(
      '/api/version/upgrade',
      { method: 'POST', body: JSON.stringify(tag ? { tag } : {}) },
    ),

  // Provider types
  providerTypes: () =>
    request<{ types: ProviderTypeInfo[] }>('/api/provider-types').then((r) => r.types),

  // Dashboard
  dashboard: () => request<{ services: DashboardService[]; updated_at?: string }>('/api/dashboard'),
  refreshDashboard: () =>
    request<{ services: DashboardService[]; updated_at?: string }>('/api/dashboard/refresh', {
      method: 'POST',
    }),
  refreshService: (id: string) =>
    request<DashboardService>(`/api/model-services/${id}/refresh`, { method: 'POST' }),
  rawQuota: (id: string) =>
    request<{
      service_id: string;
      service_name: string;
      unsupported: boolean;
      raw?: RawResponseLite;
      last_quota_at?: string;
      last_quota_error?: string;
    }>(`/api/model-services/${id}/raw-quota`),

  // Model services
  listServices: () => request<{ services: ModelService[] }>('/api/model-services').then((r) => r.services),
  getService: (id: string) => request<ModelService>(`/api/model-services/${id}`),
  createService: (body: unknown) =>
    request<ModelService>('/api/model-services', { method: 'POST', body: JSON.stringify(body) }),
  updateService: (id: string, body: unknown) =>
    request<ModelService>(`/api/model-services/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteService: (id: string) => request<unknown>(`/api/model-services/${id}`, { method: 'DELETE' }),
  refreshModels: (id: string) =>
    request<{ models: string[] }>(`/api/model-services/${id}/models/refresh`, { method: 'POST' }),
  adhocModels: (code: string, credential: Record<string, string>) =>
    request<{ models: string[] }>(`/api/providers/${code}/models`, {
      method: 'POST',
      body: JSON.stringify({ credential }),
    }),
  reorderServices: (ids: string[]) =>
    request<unknown>('/api/model-services/order', { method: 'PUT', body: JSON.stringify({ ids }) }),

  // Tasks
  listTasks: () => request<{ tasks: Task[] }>('/api/tasks').then((r) => r.tasks),
  createTask: (body: unknown) => request<Task>('/api/tasks', { method: 'POST', body: JSON.stringify(body) }),
  updateTask: (id: string, body: unknown) =>
    request<Task>(`/api/tasks/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteTask: (id: string) => request<unknown>(`/api/tasks/${id}`, { method: 'DELETE' }),
  runTask: (id: string) => request<unknown>(`/api/tasks/${id}/run`, { method: 'POST' }),
  taskExecutions: (id: string) =>
    request<{ executions: Execution[] }>(`/api/tasks/${id}/executions`).then((r) => r.executions),
  deleteTaskExecutions: (id: string) =>
    request<unknown>(`/api/tasks/${id}/executions`, { method: 'DELETE' }),
  execution: (id: string) =>
    request<{ execution: Execution; attempts: ExecutionAttempt[] }>(`/api/executions/${id}`),

  // Test logs
  testLogs: (id: string) =>
    request<{ logs: TestLog[] }>(`/api/model-services/${id}/test-logs`).then((r) => r.logs),
  deleteTestLogs: (id: string) =>
    request<unknown>(`/api/model-services/${id}/test-logs`, { method: 'DELETE' }),

  // Log cleanup
  cleanupLogs: (olderThanDays: number) =>
    request<{ deleted_executions: number; deleted_test_logs: number }>('/api/logs/cleanup', {
      method: 'POST',
      body: JSON.stringify({ older_than_days: olderThanDays }),
    }),

  // Notifications
  listChannels: () =>
    request<{ channels: NotificationChannel[] }>('/api/notifications').then((r) => r.channels),
  createChannel: (body: unknown) =>
    request<NotificationChannel>('/api/notifications', { method: 'POST', body: JSON.stringify(body) }),
  updateChannel: (id: string, body: unknown) =>
    request<NotificationChannel>(`/api/notifications/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteChannel: (id: string) => request<unknown>(`/api/notifications/${id}`, { method: 'DELETE' }),
  testChannel: (id: string) => request<unknown>(`/api/notifications/${id}/test`, { method: 'POST' }),

  // Settings
  getProxy: () => request<ProxyConfig>('/api/settings/proxy'),
  putProxy: (body: ProxyConfig) =>
    request<ProxyConfig>('/api/settings/proxy', { method: 'PUT', body: JSON.stringify(body) }),
  testProxy: (body: ProxyConfig) =>
    request<{ ok: boolean; status?: number; latency_ms: number; error?: string }>(
      '/api/settings/proxy/test',
      { method: 'POST', body: JSON.stringify(body) },
    ),

  // Config backup: export / import
  importConfig: (backupJSON: string) =>
    request<Record<string, number>>('/api/settings/import', {
      method: 'POST',
      body: backupJSON,
    }),

  // WebDAV sync
  getWebdav: () =>
    request<{
      server: string;
      username: string;
      path: string;
      password_set: boolean;
      last_sync: string;
    }>('/api/settings/webdav'),
  putWebdav: (body: { server: string; username: string; password?: string; path: string }) =>
    request<unknown>('/api/settings/webdav', { method: 'PUT', body: JSON.stringify(body) }),
  testWebdav: () =>
    request<{ ok: boolean; error?: string | null; latency_ms: number }>(
      '/api/settings/webdav/test',
      { method: 'POST' },
    ),
  webdavSync: () =>
    request<{ ok: boolean; size: number; synced_at: string }>('/api/settings/webdav/sync', {
      method: 'POST',
    }),
  webdavRestore: () =>
    request<Record<string, number>>('/api/settings/webdav/restore', { method: 'POST' }),
};

/** Downloads the config backup as a JSON file. */
export async function downloadConfigBackup(): Promise<void> {
  const res = await fetch('/api/settings/export');
  if (!res.ok) throw new Error(`导出失败: HTTP ${res.status}`);
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `ai-subscription-keeper-backup-${new Date()
    .toISOString()
    .slice(0, 10)}.json`;
  a.click();
  URL.revokeObjectURL(url);
}

export interface RawResponseLite {
  url: string;
  method: string;
  http_status: number;
  queried_at: string;
  duration_ms: number;
  body?: unknown;
  error?: string;
}

export interface TestStreamEvent {
  type: 'meta' | 'delta' | 'done' | 'error';
  model?: string;
  prompt?: string;
  content?: string;
  result?: StreamResult;
  error?: string;
}

/** Streams a model test over SSE and invokes onEvent per parsed event. */
export async function streamModelTest(
  serviceId: string,
  body: { model?: string; prompt?: string },
  onEvent: (ev: TestStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(`/api/model-services/${serviceId}/test`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  });
  if (!res.ok || !res.body) {
    let msg = `HTTP ${res.status}`;
    try {
      const j = await res.json();
      if (j?.error) msg = j.error;
    } catch {
      /* ignore */
    }
    onEvent({ type: 'error', error: msg });
    return;
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const parts = buffer.split('\n\n');
    buffer = parts.pop() ?? '';
    for (const part of parts) {
      const line = part.split('\n').find((l) => l.startsWith('data:'));
      if (!line) continue;
      try {
        onEvent(JSON.parse(line.slice(5).trim()));
      } catch {
        /* skip malformed */
      }
    }
  }
}
