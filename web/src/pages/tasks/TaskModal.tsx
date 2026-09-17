import { useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Col,
  Form,
  Input,
  InputNumber,
  Modal,
  Popover,
  Row,
  Select,
  Space,
  Switch,
  TimePicker,
  Typography,
  message,
} from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { api } from '../../api';
import type { ModelService, NotificationChannel, Task } from '../../types';

const { Text } = Typography;

export interface WebhookCfg {
  method: string;
  url: string;
  headers?: string;
  body?: string;
}

export interface TaskFormValues {
  name: string;
  type: 'warmup' | 'webhook' | 'reminder';
  model_service_id?: string;
  model?: string;
  prompt?: string;
  cron: string;
  timezone: string;
  retry_count: number;
  retry_interval_min: number;
  notification_channel_ids?: string[];
  enabled?: boolean;
  webhook_method?: string;
  webhook_url?: string;
  webhook_headers?: string;
  webhook_body?: string;
}

type ScheduleMode = 'daily' | 'weekly' | 'custom';

const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];
const WEEKDAY_VALUES = ['0', '1', '2', '3', '4', '5', '6'];

/** Parse a 5-field cron expression into a schedule mode. */
function parseScheduleMode(cron: string): { mode: ScheduleMode; time: string; weekdays: string[] } {
  const parts = cron.trim().split(/\s+/);
  if (parts.length !== 5) return { mode: 'custom', time: '06:00', weekdays: ['1'] };
  const [min, hour, dom, mon, dow] = parts;
  if (dom === '*' && mon === '*') {
    const time = `${hour.padStart(2, '0')}:${min.padStart(2, '0')}`;
    if (dow === '*') return { mode: 'daily', time, weekdays: ['1'] };
    if (/^[0-6](,[0-6])*$/.test(dow) || /^[0-6]-[0-6]$/.test(dow)) {
      const weekdays = dow.includes('-') ? expandRange(dow) : dow.split(',');
      return { mode: 'weekly', time, weekdays };
    }
  }
  return { mode: 'custom', time: '06:00', weekdays: ['1'] };
}

function expandRange(range: string): string[] {
  const [start, end] = range.split('-').map(Number);
  const out: string[] = [];
  for (let i = start; i <= end; i++) out.push(String(i));
  return out;
}

function buildCron(mode: ScheduleMode, time: string, weekdays: string[]): string {
  const [h, m] = time.split(':').map((x) => x.padStart(2, '0'));
  if (mode === 'daily') return `${Number(m)} ${Number(h)} * * *`;
  if (mode === 'weekly') return `${Number(m)} ${Number(h)} * * ${weekdays.join(',')}`;
  return `${Number(m)} ${Number(h)} * * *`;
}

const CRON_HELP = (
  <div style={{ maxWidth: 320 }}>
    <Typography.Title level={5} style={{ marginBottom: 8 }}>Cron 表达式格式</Typography.Title>
    <Text style={{ fontSize: 13 }}>
      <code>分 时 日 月 周</code>
      <br />
      <br />
      <strong>分</strong>：0-59（<code>*</code> 每分钟，<code>*/5</code> 每 5 分钟）<br />
      <strong>时</strong>：0-23<br />
      <strong>日</strong>：1-31<br />
      <strong>月</strong>：1-12<br />
      <strong>周</strong>：0-6（0=周日，1=周一）<br />
      <br />
      示例：<br />
      <code>0 6 * * *</code> → 每天 6:00<br />
      <code>0 9 * * 1-5</code> → 工作日 9:00<br />
      <code>*/30 * * * *</code> → 每 30 分钟<br />
      <code>0 0 * * 0</code> → 每周日 0:00
    </Text>
  </div>
);

export default function TaskModal({
  open,
  editing,
  services,
  channels,
  onClose,
  onSaved,
}: {
  open: boolean;
  editing: Task | null;
  services: ModelService[];
  channels: NotificationChannel[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [form] = Form.useForm<TaskFormValues>();
  const [saving, setSaving] = useState(false);
  const [scheduleMode, setScheduleMode] = useState<ScheduleMode>('daily');
  const [scheduleTime, setScheduleTime] = useState(dayjs('06:00', 'HH:mm'));
  const [scheduleWeekdays, setScheduleWeekdays] = useState<string[]>(['1']);
  const taskType = Form.useWatch('type', form);
  const serviceId = Form.useWatch('model_service_id', form);

  const svc = services.find((s) => s.id === serviceId);
  const svcModels = (svc?.models ?? []).map((m) => ({ value: m, label: m }));

  // 只显示订阅类型且已启用的模型服务（按量计费不可预热）
  const warmupServices = services.filter(
    (s) => s.enabled && (s.billing_type ?? 'subscription') !== 'pay_as_you_go',
  );

  useEffect(() => {
    if (open) {
      form.resetFields();
      if (editing) {
        let wh: Partial<WebhookCfg> = {};
        try {
          wh = JSON.parse(editing.webhook_config || '{}');
        } catch {
          /* ignore */
        }
        form.setFieldsValue({
          name: editing.name,
          type: editing.type,
          model_service_id: editing.model_service_id || undefined,
          model: editing.model || undefined,
          prompt: editing.prompt,
          cron: editing.cron,
          timezone: editing.timezone,
          retry_count: editing.retry_count,
          retry_interval_min: editing.retry_interval_min,
          notification_channel_ids: editing.notification_channel_ids ?? [],
          enabled: editing.enabled,
          webhook_method: wh.method || 'POST',
          webhook_url: wh.url,
          webhook_headers: wh.headers
            ? Object.entries(wh.headers)
                .map(([k, v]) => `${k}: ${v}`)
                .join('\n')
            : '',
          webhook_body: wh.body,
        });
        // Parse existing cron into schedule mode
        const parsed = parseScheduleMode(editing.cron);
        setScheduleMode(parsed.mode);
        setScheduleTime(dayjs(parsed.time, 'HH:mm'));
        setScheduleWeekdays(parsed.weekdays);
      } else {
        form.setFieldsValue({
          type: 'warmup',
          prompt: 'hi',
          cron: '0 6 * * *',
          timezone: 'Asia/Shanghai',
          retry_count: 3,
          retry_interval_min: 5,
          enabled: true,
          webhook_method: 'POST',
        });
        setScheduleMode('daily');
        setScheduleTime(dayjs('06:00', 'HH:mm'));
        setScheduleWeekdays(['1']);
      }
    }
  }, [open, editing, form]);

  // Sync cron field when schedule mode/time/weekdays change
  useEffect(() => {
    const cron = buildCron(scheduleMode, scheduleTime.format('HH:mm'), scheduleWeekdays);
    form.setFieldValue('cron', cron);
  }, [scheduleMode, scheduleTime, scheduleWeekdays]); // eslint-disable-line react-hooks/exhaustive-deps

  // 选择模型服务后自动填充默认预热模型
  useEffect(() => {
    if (!editing && svc?.default_warmup_model) {
      form.setFieldValue('model', svc.default_warmup_model);
    }
  }, [serviceId]); // eslint-disable-line react-hooks/exhaustive-deps

  async function handleSave() {
    const values = await form.validateFields();
    setSaving(true);
    try {
      let webhookConfig = '{}';
      if (values.type === 'webhook') {
        const headers: Record<string, string> = {};
        for (const line of (values.webhook_headers ?? '').split('\n')) {
          const idx = line.indexOf(':');
          if (idx > 0) {
            headers[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
          }
        }
        webhookConfig = JSON.stringify({
          method: values.webhook_method || 'POST',
          url: values.webhook_url,
          headers,
          body: values.webhook_body ?? '',
        });
      }
      const body = {
        name: values.name,
        type: values.type,
        model_service_id: values.type === 'warmup' ? values.model_service_id : '',
        model: values.type === 'warmup' ? values.model : '',
        prompt: values.type === 'reminder' ? values.prompt : values.prompt || 'hi',
        cron: values.cron,
        timezone: values.timezone || 'Asia/Shanghai',
        webhook_config: webhookConfig,
        retry_count: values.retry_count ?? 0,
        retry_interval_min: values.retry_interval_min ?? 5,
        notification_channel_ids: values.notification_channel_ids ?? [],
        enabled: values.enabled ?? true,
      };
      if (editing) {
        await api.updateTask(editing.id, body);
        message.success('已保存');
      } else {
        await api.createTask(body);
        message.success('已创建');
      }
      onSaved();
      onClose();
    } catch (e) {
      message.error(`保存失败: ${(e as Error).message}`);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      title={editing ? `编辑任务 - ${editing.name}` : '新增定时任务'}
      open={open}
      onCancel={onClose}
      onOk={handleSave}
      confirmLoading={saving}
      okText="保存"
      width={680}
      destroyOnClose
    >
      <Form form={form} layout="vertical">
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item name="name" label="任务名称" rules={[{ required: true }]}>
              <Input placeholder="智谱早间预热" />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item name="type" label="任务类型" rules={[{ required: true }]}>
              <Select
                disabled={!!editing}
                options={[
                  { value: 'warmup', label: '模型预热' },
                  { value: 'webhook', label: 'Webhook' },
                  { value: 'reminder', label: '定时提醒' },
                ]}
              />
            </Form.Item>
          </Col>
        </Row>

        {taskType === 'warmup' ? (
          <>
            <Row gutter={16}>
              <Col span={12}>
                <Form.Item
                  name="model_service_id"
                  label="模型服务"
                  rules={[{ required: true, message: '请选择模型服务' }]}
                  extra={
                    warmupServices.length === 0 ? (
                      <Text type="warning" style={{ fontSize: 12 }}>
                        暂无可选的订阅类型模型服务（按量计费不可预热）
                      </Text>
                    ) : undefined
                  }
                >
                  <Select
                    showSearch
                    optionFilterProp="label"
                    options={warmupServices.map((s) => ({ value: s.id, label: s.name }))}
                  />
                </Form.Item>
              </Col>
              <Col span={12}>
                <Form.Item
                  name="model"
                  label="预热模型"
                  rules={[{ required: true, message: '请选择预热模型' }]}
                  extra={svc?.default_warmup_model ? `服务默认：${svc.default_warmup_model}` : undefined}
                >
                  <Select
                    showSearch
                    allowClear
                    options={svcModels}
                    placeholder="选择或输入模型"
                  />
                </Form.Item>
              </Col>
            </Row>
            <Form.Item name="prompt" label="提示词">
              <Input placeholder="hi" />
            </Form.Item>
          </>
        ) : taskType === 'reminder' ? (
          <>
            <Form.Item
              name="prompt"
              label="提醒文案"
              rules={[{ required: true, message: '请填写提醒文案' }]}
              help="到点后该文案将原样推送到下方选中的通知渠道"
            >
              <Input.TextArea rows={3} placeholder="记得检查各订阅的额度与续费时间～" />
            </Form.Item>
          </>
        ) : (
          <>
            <Row gutter={16}>
              <Col span={6}>
                <Form.Item name="webhook_method" label="请求方法">
                  <Select
                    options={['GET', 'POST', 'PUT', 'DELETE', 'PATCH'].map((m) => ({
                      value: m,
                      label: m,
                    }))}
                  />
                </Form.Item>
              </Col>
              <Col span={18}>
                <Form.Item
                  name="webhook_url"
                  label="URL"
                  rules={[{ required: true, message: '请输入 URL' }]}
                >
                  <Input placeholder="https://example.com/api/build" />
                </Form.Item>
              </Col>
            </Row>
            <Form.Item
              name="webhook_headers"
              label="Headers（每行一个，格式 Key: Value）"
            >
              <Input.TextArea
                rows={2}
                placeholder={'Authorization: Bearer xxx'}
              />
            </Form.Item>
            <Form.Item name="webhook_body" label="Body（支持模板变量）">
              <Input.TextArea
                rows={4}
                placeholder={`{"date": "{{date}}", "task": "{{task.name}}"}`}
              />
            </Form.Item>
            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 16 }}
              message="支持变量：{{date}} {{time}} {{timestamp}} {{task.name}} {{execution.id}} {{status}} {{attempt}}"
            />
          </>
        )}

        <Row gutter={16} align="bottom">
          <Col span={6}>
            <Form.Item label="执行时间">
              <Select
                value={scheduleMode}
                onChange={(v) => setScheduleMode(v)}
                options={[
                  { value: 'daily', label: '每天' },
                  { value: 'weekly', label: '每周' },
                  { value: 'custom', label: '自定义 Cron' },
                ]}
              />
            </Form.Item>
          </Col>
          {scheduleMode !== 'custom' && (
            <Col span={5}>
              <Form.Item label="时间">
                <TimePicker
                  value={scheduleTime}
                  onChange={(t) => t && setScheduleTime(t)}
                  format="HH:mm"
                  minuteStep={5}
                  style={{ width: '100%' }}
                />
              </Form.Item>
            </Col>
          )}
          {scheduleMode === 'weekly' && (
            <Col span={9}>
              <Form.Item label="星期">
                <Select
                  mode="multiple"
                  value={scheduleWeekdays}
                  onChange={(v) => setScheduleWeekdays(v)}
                  options={WEEKDAY_VALUES.map((v, i) => ({ value: v, label: WEEKDAYS[i] }))}
                />
              </Form.Item>
            </Col>
          )}
          {scheduleMode === 'custom' && (
            <Col span={14}>
              <Form.Item
                name="cron"
                label={
                  <Space>
                    Cron 表达式
                    <Popover content={CRON_HELP} title="Cron 格式说明">
                      <QuestionCircleOutlined style={{ color: 'rgba(255,255,255,0.45)' }} />
                    </Popover>
                  </Space>
                }
                rules={[{ required: true, message: '请输入 Cron 表达式' }]}
              >
                <Input placeholder="0 6 * * *" />
              </Form.Item>
            </Col>
          )}
          {scheduleMode !== 'custom' && (
            <Col span={4}>
              <Form.Item name="cron" hidden>
                <Input />
              </Form.Item>
            </Col>
          )}
        </Row>

        <Row gutter={16}>
          <Col span={taskType === 'reminder' ? 12 : 8}>
            <Form.Item name="timezone" label="时区">
              <Select
                showSearch
                options={[
                  'Asia/Shanghai',
                  'UTC',
                  'America/New_York',
                  'Europe/London',
                  'Asia/Tokyo',
                ].map((t) => ({ value: t, label: t }))}
              />
            </Form.Item>
          </Col>
          {taskType !== 'reminder' && (
            <>
              <Col span={4}>
                <Form.Item name="retry_count" label="失败重试次数">
                  <InputNumber min={0} max={10} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col span={4}>
                <Form.Item name="retry_interval_min" label="重试间隔(分)">
                  <InputNumber min={1} max={1440} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
            </>
          )}
        </Row>

        <Row gutter={16}>
          <Col span={12}>
            <Form.Item
              name="notification_channel_ids"
              label="通知渠道（可多选）"
              rules={taskType === 'reminder' ? [{ required: true, message: '定时提醒必须选择通知渠道' }] : []}
              help={taskType === 'reminder' ? '提醒文案将推送到所有选中渠道' : '执行结束后同时推送到所有选中渠道'}
            >
              <Select
                mode="multiple"
                allowClear
                placeholder="不通知"
                options={channels
                  .filter((c) => c.enabled)
                  .map((c) => ({ value: c.id, label: c.name }))}
              />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item name="enabled" label="启用" valuePropName="checked">
              <Switch />
            </Form.Item>
          </Col>
        </Row>
      </Form>
    </Modal>
  );
}
