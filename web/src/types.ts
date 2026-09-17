export interface ProviderField {
  name: string;
  label: string;
  type: string;
  required: boolean;
  placeholder?: string;
  help?: string;
}

export interface ProviderTypeInfo {
  code: string;
  name: string;
  fields: ProviderField[];
  supports_quota: boolean;
  auth_note?: string;
  official?: boolean;
  logo?: string;
}

export interface QuotaDimension {
  code: string;
  display_name: string;
  used_percentage?: number;
  reset_at?: string;
}

export interface BalanceInfo {
  amount: number;
  currency: string;
}

export interface UsageInfo {
  today_spend?: number;
  month_spend?: number;
  requests?: number;
  tokens?: number;
}

export interface QuotaMetric {
  label: string;
  value: string;
}

export interface QuotaInfo {
  dimensions?: QuotaDimension[];
  balance?: BalanceInfo;
  usage?: UsageInfo;
  extra?: QuotaMetric[];
}

export interface RawResponse {
  url: string;
  method: string;
  http_status: number;
  queried_at: string;
  duration_ms: number;
  body?: unknown;
  error?: string;
}

export interface DashboardService {
  id: string;
  name: string;
  provider_type: string;
  provider_name: string;
  provider_logo?: string;
  provider_website?: string;
  provider_usage_url?: string;
  enabled: boolean;
  status: 'ok' | 'error' | 'unknown';
  quota?: QuotaInfo;
  raw?: RawResponse;
  unsupported_quota: boolean;
  last_quota_at?: string;
  last_quota_error?: string;
  default_test_model: string;
  sort_order: number;
}

export interface ModelService {
  id: string;
  name: string;
  provider_type: string;
  credential: Record<string, string>;
  models: string[];
  default_warmup_model: string;
  default_test_model: string;
  default_test_prompt: string;
  quota_labels: Record<string, string>;
  billing_type: 'subscription' | 'pay_as_you_go';
  sort_order: number;
  enabled: boolean;
  last_quota_at?: string;
  last_quota_error?: string;
  created_at: string;
  updated_at: string;
}

export interface Task {
  id: string;
  name: string;
  type: 'warmup' | 'webhook';
  model_service_id: string;
  model: string;
  prompt: string;
  cron: string;
  timezone: string;
  webhook_config: string;
  retry_count: number;
  retry_interval_min: number;
  notification_channel_ids: string[];
  enabled: boolean;
  created_at: string;
  updated_at: string;
  model_service_name?: string;
  notification_channel_names?: string[];
  running?: boolean;
  next_run_at?: string;
  last_execution?: {
    status: string;
    finished_at?: string;
    error: string;
  };
}

export interface Execution {
  id: string;
  task_id: string;
  status: 'running' | 'success' | 'failed';
  started_at: string;
  finished_at?: string;
  attempt_count: number;
  result: string;
  error: string;
  created_at: string;
}

export interface ExecutionAttempt {
  id: string;
  execution_id: string;
  attempt: number;
  started_at: string;
  finished_at?: string;
  model_status: string;
  quota_status: string;
  http_status: number;
  result: string;
  error: string;
}

export interface NotificationChannel {
  id: string;
  name: string;
  type: 'dingtalk' | 'webhook' | 'feishu' | 'telegram' | 'wecom' | 'ntfy' | 'bark' | 'gotify' | 'email';
  webhook?: string;
  url?: string;
  method?: string;
  secret_set?: boolean;
  bot_token?: string;
  chat_id?: string;
  topic?: string;
  server?: string;
  device_key?: string;
  app_token?: string;
  smtp_host?: string;
  smtp_port?: number;
  smtp_username?: string;
  from?: string;
  to?: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface TestLog {
  id: string;
  service_id: string;
  model: string;
  prompt: string;
  status: 'running' | 'success' | 'failed';
  result: string;
  error: string;
  started_at: string;
  finished_at?: string;
  created_at: string;
}

export interface ProxyConfig {
  mode: 'none' | 'proxy';
  url: string;
}

export interface StreamUsage {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export interface StreamResult {
  model: string;
  content: string;
  duration_ms: number;
  usage?: StreamUsage;
}
