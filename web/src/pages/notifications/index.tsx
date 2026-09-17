import { useCallback, useEffect, useState } from 'react';
import {
  Button,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  QuestionCircleOutlined,
  SendOutlined,
} from '@ant-design/icons';
import { api } from '../../api';
import type { NotificationChannel } from '../../types';

interface ChannelFormValues {
  name: string;
  type: 'dingtalk' | 'webhook' | 'feishu' | 'telegram' | 'wecom' | 'ntfy' | 'bark' | 'gotify' | 'email';
  webhook?: string;
  secret?: string;
  url?: string;
  method?: string;
  bot_token?: string;
  chat_id?: string;
  topic?: string;
  server?: string;
  device_key?: string;
  app_token?: string;
  email_provider?: string;
  smtp_host?: string;
  smtp_port?: number;
  smtp_username?: string;
  smtp_password?: string;
  from?: string;
  to?: string;
  use_tls?: boolean;
  enabled?: boolean;
}

const CHANNEL_TYPES: { value: string; label: string; color: string }[] = [
  { value: 'dingtalk', label: '钉钉机器人', color: 'blue' },
  { value: 'feishu', label: '飞书机器人', color: 'green' },
  { value: 'wecom', label: '企业微信机器人', color: 'cyan' },
  { value: 'telegram', label: 'Telegram', color: 'geekblue' },
  { value: 'ntfy', label: 'ntfy 推送', color: 'orange' },
  { value: 'bark', label: 'Bark (iOS)', color: 'gold' },
  { value: 'gotify', label: 'Gotify (自建)', color: 'lime' },
  { value: 'email', label: '邮件 (SMTP)', color: 'volcano' },
  { value: 'webhook', label: 'Webhook', color: 'purple' },
];

// ---------------------------------------------------------------------------
// 邮箱快捷模板：选择后自动填充 SMTP 服务器 / 端口 / 加密方式
// ---------------------------------------------------------------------------
interface EmailPreset {
  label: string;
  host: string;
  port: number;
  useTls: boolean;
  passwordLabel: string;
  passwordHint: string;
}

const EMAIL_PRESETS: Record<string, EmailPreset> = {
  qq: {
    label: 'QQ 邮箱',
    host: 'smtp.qq.com',
    port: 465,
    useTls: true,
    passwordLabel: 'SMTP 授权码',
    passwordHint: '不是 QQ 登录密码。在 QQ 邮箱「设置 → 账户 → POP3/IMAP/SMTP 服务」开启后生成的授权码',
  },
  '163': {
    label: '163 邮箱',
    host: 'smtp.163.com',
    port: 465,
    useTls: true,
    passwordLabel: 'SMTP 授权码',
    passwordHint: '在 163 邮箱「设置 → POP3/SMTP/IMAP」开启服务后生成的授权码',
  },
  gmail: {
    label: 'Gmail',
    host: 'smtp.gmail.com',
    port: 465,
    useTls: true,
    passwordLabel: '应用专用密码',
    passwordHint: '需先开启两步验证，再到 Google 账户「安全 → 应用专用密码」创建',
  },
  outlook: {
    label: 'Outlook',
    host: 'smtp.office365.com',
    port: 587,
    useTls: false,
    passwordLabel: '账户密码',
    passwordHint: 'Microsoft 账户密码；若开启了两步验证需使用应用密码',
  },
  custom: {
    label: '自定义 SMTP',
    host: '',
    port: 465,
    useTls: true,
    passwordLabel: '密码 / 授权码',
    passwordHint: 'SMTP 登录密码或授权码',
  },
};

/** 根据已保存的 SMTP 服务器地址反推邮箱模板。 */
function detectEmailProvider(host?: string): string {
  if (!host) return 'qq';
  for (const [key, p] of Object.entries(EMAIL_PRESETS)) {
    if (key !== 'custom' && p.host === host) return key;
  }
  return 'custom';
}

function randomTopic(): string {
  const suffix = Math.random().toString(36).slice(2, 10);
  return `ai-keeper-${suffix}`;
}

// ---------------------------------------------------------------------------
// 各渠道配置指引
// ---------------------------------------------------------------------------
const helpTextStyle: React.CSSProperties = { fontSize: 13, marginBottom: 8 };

const CHANNEL_HELP: Record<string, React.ReactNode> = {
  dingtalk: (
    <div style={{ maxWidth: 360 }}>
      <Typography.Title level={5} style={{ marginBottom: 8 }}>钉钉机器人配置指引</Typography.Title>
      <Typography.Paragraph style={helpTextStyle}>
        <strong>获取 Webhook 步骤：</strong>
        <ol style={{ paddingLeft: 20, margin: '4px 0' }}>
          <li>打开目标钉钉群 → 右上角「群设置」→「智能群助手」</li>
          <li>「添加机器人」→ 选择「自定义（通过 Webhook 接入）」</li>
          <li>安全设置勾选「加签」，保存后获得 <Typography.Text code>Secret</Typography.Text></li>
          <li>复制 Webhook 地址和 Secret 填入左侧表单</li>
        </ol>
      </Typography.Paragraph>
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
        Webhook 形如 https://oapi.dingtalk.com/robot/send?access_token=xxx；Secret 即加签密钥（SEC 开头）。
      </Typography.Paragraph>
    </div>
  ),
  feishu: (
    <div style={{ maxWidth: 360 }}>
      <Typography.Title level={5} style={{ marginBottom: 8 }}>飞书机器人配置指引</Typography.Title>
      <Typography.Paragraph style={helpTextStyle}>
        <strong>获取 Webhook 步骤：</strong>
        <ol style={{ paddingLeft: 20, margin: '4px 0' }}>
          <li>打开目标飞书群 →「设置」→「群机器人」</li>
          <li>「添加机器人」→ 选择「自定义机器人」</li>
          <li>可选开启「签名校验」，获得 Secret</li>
          <li>复制 Webhook 地址填入左侧表单</li>
        </ol>
      </Typography.Paragraph>
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
        Webhook 形如 https://open.feishu.cn/open-apis/bot/v2/hook/xxx。
      </Typography.Paragraph>
    </div>
  ),
  wecom: (
    <div style={{ maxWidth: 360 }}>
      <Typography.Title level={5} style={{ marginBottom: 8 }}>企业微信机器人配置指引</Typography.Title>
      <Typography.Paragraph style={helpTextStyle}>
        <strong>获取 Webhook 步骤：</strong>
        <ol style={{ paddingLeft: 20, margin: '4px 0' }}>
          <li>打开目标企业微信群 → 右键群聊 →「添加群机器人」</li>
          <li>「新建一个机器人」，输入名称</li>
          <li>复制 Webhook 地址填入左侧表单</li>
        </ol>
      </Typography.Paragraph>
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
        Webhook 形如 https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx。企业微信机器人无需签名。
      </Typography.Paragraph>
    </div>
  ),
  telegram: (
    <div style={{ maxWidth: 380 }}>
      <Typography.Title level={5} style={{ marginBottom: 8 }}>Telegram Bot 配置指引</Typography.Title>
      <Typography.Paragraph style={helpTextStyle}>
        <strong>1. 创建 Bot 获取 Token：</strong>
        <ol style={{ paddingLeft: 20, margin: '4px 0' }}>
          <li>在 Telegram 搜索 <Typography.Text code>@BotFather</Typography.Text></li>
          <li>发送 <Typography.Text code>/newbot</Typography.Text>，按提示取名</li>
          <li>获得 Bot Token（形如 <Typography.Text code>123456:ABC-DEF...</Typography.Text>）</li>
        </ol>
      </Typography.Paragraph>
      <Typography.Paragraph style={helpTextStyle}>
        <strong>2. 获取 Chat ID：</strong>
        <ol style={{ paddingLeft: 20, margin: '4px 0' }}>
          <li>先给你创建的 Bot 发一条消息（任意内容）</li>
          <li>浏览器访问 <Typography.Text code>https://api.telegram.org/bot&lt;Token&gt;/getUpdates</Typography.Text></li>
          <li>在返回 JSON 中找到 <Typography.Text code>chat.id</Typography.Text></li>
        </ol>
      </Typography.Paragraph>
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
        推送到频道：把 Bot 加入频道为管理员，Chat ID 填 <Typography.Text code>@频道名</Typography.Text>；
        群组 Chat ID 为负数（-100 开头）。注意：国内网络可能需要代理。
      </Typography.Paragraph>
    </div>
  ),
  ntfy: (
    <div style={{ maxWidth: 360 }}>
      <Typography.Title level={5} style={{ marginBottom: 8 }}>ntfy 使用说明</Typography.Title>
      <Typography.Paragraph style={helpTextStyle}>
        ntfy 是开源推送通知服务，默认使用官方免费服务{' '}
        <Typography.Text code>https://ntfy.sh</Typography.Text>，<strong>无需注册账号</strong>。
      </Typography.Paragraph>
      <Typography.Paragraph style={helpTextStyle}>
        <strong>使用步骤：</strong>
        <ol style={{ paddingLeft: 20, margin: '4px 0' }}>
          <li>手机安装 ntfy App（iOS / Android 均有官方 App）</li>
          <li>App 中点「+ 订阅主题」，输入左侧填写的相同主题名</li>
          <li>主题无需预先创建，订阅即生效，通知秒到手机</li>
        </ol>
      </Typography.Paragraph>
      <Typography.Paragraph style={helpTextStyle}>
        <strong>注意事项：</strong>
        <ul style={{ paddingLeft: 20, margin: '4px 0' }}>
          <li>公共服务器上<strong>主题名即密码</strong>：名称公开可见，请点「随机」按钮生成不可猜测的主题名，避免他人订阅到你的通知</li>
          <li>官方服务对公共主题有每日消息频率限制</li>
          <li>注重隐私可自建 ntfy 服务器，在「服务器地址」填写自建地址</li>
        </ul>
      </Typography.Paragraph>
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
        官方文档：
        <a href="https://docs.ntfy.sh/publish/" target="_blank" rel="noreferrer">docs.ntfy.sh</a>
        {' ｜ '}
        App 下载：
        <a href="https://ntfy.sh/docs/subscribe/phone/" target="_blank" rel="noreferrer">ntfy.sh App</a>
      </Typography.Paragraph>
    </div>
  ),
  bark: (
    <div style={{ maxWidth: 360 }}>
      <Typography.Title level={5} style={{ marginBottom: 8 }}>Bark 配置指引</Typography.Title>
      <Typography.Paragraph style={helpTextStyle}>
        Bark 是 iOS 专用推送 App，默认使用官方服务{' '}
        <Typography.Text code>https://api.day.app</Typography.Text>，消息直达 iPhone 系统通知。
      </Typography.Paragraph>
      <Typography.Paragraph style={helpTextStyle}>
        <strong>使用步骤：</strong>
        <ol style={{ paddingLeft: 20, margin: '4px 0' }}>
          <li>App Store 搜索「Bark」安装</li>
          <li>打开 App，首页会显示形如 <Typography.Text code>https://api.day.app/这里是你Key/推送内容</Typography.Text></li>
          <li>复制 URL 中间的<strong>设备 Key</strong> 填入左侧表单</li>
        </ol>
      </Typography.Paragraph>
      <Typography.Paragraph style={helpTextStyle}>
        <strong>注意：</strong>设备 Key 等同于密码，泄露后他人可向你推送消息；怀疑泄露时在 Bark App 内更换 Key。
      </Typography.Paragraph>
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
        官方文档：
        <a href="https://bark.day.app/#/tutorial" target="_blank" rel="noreferrer">bark.day.app</a>
        {' ｜ '}
        自建服务器在「服务器地址」填写。
      </Typography.Paragraph>
    </div>
  ),
  gotify: (
    <div style={{ maxWidth: 360 }}>
      <Typography.Title level={5} style={{ marginBottom: 8 }}>Gotify 配置指引</Typography.Title>
      <Typography.Paragraph style={helpTextStyle}>
        Gotify 是<strong>自托管</strong>推送通知服务（需自己部署服务器），支持 Android / iOS / Web 客户端，消息通过 WebSocket 实时到达。
      </Typography.Paragraph>
      <Typography.Paragraph style={helpTextStyle}>
        <strong>使用步骤：</strong>
        <ol style={{ paddingLeft: 20, margin: '4px 0' }}>
          <li>自建 Gotify 服务器（Docker 一行命令：<Typography.Text code>docker run -p 80:80 gotify/server</Typography.Text>）</li>
          <li>手机安装 Gotify 客户端，登录你的服务器</li>
          <li>Web 端「APPS」→「CREATE APPLICATION」创建应用</li>
          <li>复制<strong>应用令牌</strong>（Token）填入左侧表单</li>
        </ol>
      </Typography.Paragraph>
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
        官方文档：
        <a href="https://gotify.net/docs/pushmsg" target="_blank" rel="noreferrer">gotify.net/docs/pushmsg</a>
      </Typography.Paragraph>
    </div>
  ),
  email: (
    <div style={{ maxWidth: 380 }}>
      <Typography.Title level={5} style={{ marginBottom: 8 }}>邮件通知配置指引</Typography.Title>
      <Typography.Paragraph style={helpTextStyle}>
        选择邮箱类型后自动填充 SMTP 服务器，<strong>只需填写邮箱地址、授权码和收件人</strong>。
      </Typography.Paragraph>
      <Typography.Paragraph style={helpTextStyle}>
        <strong>QQ 邮箱：</strong>设置 → 账户 → POP3/IMAP/SMTP 服务 → 开启 SMTP → 生成<strong>授权码</strong>（非 QQ 密码）。
      </Typography.Paragraph>
      <Typography.Paragraph style={helpTextStyle}>
        <strong>Gmail：</strong>先开启两步验证，再到 Google 账户「安全 → 应用专用密码」创建 16 位 App Password 填入。
      </Typography.Paragraph>
      <Typography.Paragraph style={helpTextStyle}>
        <strong>163 邮箱：</strong>设置 → POP3/SMTP/IMAP → 开启服务 → 生成授权码。
      </Typography.Paragraph>
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
        企业邮箱 / 自建 SMTP 选「自定义」，填服务器地址、端口和加密方式（465=SSL，587=STARTTLS）。
      </Typography.Paragraph>
    </div>
  ),
};

function channelTypeLabel(type: string) {
  return CHANNEL_TYPES.find((t) => t.value === type)?.label ?? type;
}

function channelTypeColor(type: string) {
  return CHANNEL_TYPES.find((t) => t.value === type)?.color ?? 'default';
}

/** 渠道表单区顶部的帮助链接（点击打开侧边抽屉）。 */
function ChannelHelpLink({ type, onOpen }: { type: string; onOpen: (t: string) => void }) {
  const content = CHANNEL_HELP[type];
  if (!content) return null;
  return (
    <div style={{ marginBottom: 12 }}>
      <Typography.Link style={{ fontSize: 13 }} onClick={() => onOpen(type)}>
        <QuestionCircleOutlined /> 查看{channelTypeLabel(type)}配置指引
      </Typography.Link>
    </div>
  );
}

export default function NotificationsPage() {
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<NotificationChannel | null>(null);
  const [testing, setTesting] = useState<string | null>(null);
  const [helpType, setHelpType] = useState<string | null>(null);
  const [form] = Form.useForm<ChannelFormValues>();
  const channelType = Form.useWatch('type', form);
  const emailProvider = Form.useWatch('email_provider', form) ?? 'qq';
  const preset = EMAIL_PRESETS[emailProvider] ?? EMAIL_PRESETS.qq;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setChannels(await api.listChannels());
    } catch (e) {
      message.error(`加载失败: ${(e as Error).message}`);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (modalOpen) {
      form.resetFields();
      if (editing) {
        form.setFieldsValue({
          name: editing.name,
          type: editing.type,
          webhook: editing.webhook,
          secret: '',
          url: editing.url,
          method: editing.method || 'POST',
          bot_token: editing.bot_token,
          chat_id: editing.chat_id,
          topic: editing.topic,
          server: editing.type === 'ntfy' ? editing.server || 'https://ntfy.sh' : editing.server,
          device_key: editing.device_key,
          app_token: '',
          email_provider: detectEmailProvider(editing.smtp_host),
          smtp_host: editing.smtp_host,
          smtp_port: editing.smtp_port || 465,
          smtp_username: editing.smtp_username || editing.from,
          smtp_password: '',
          from: editing.from,
          to: editing.to,
          use_tls: (editing.smtp_port ?? 465) === 465,
          enabled: editing.enabled,
        });
      } else {
        form.setFieldsValue({
          type: 'dingtalk',
          method: 'POST',
          email_provider: 'qq',
          smtp_port: 465,
          enabled: true,
        });
      }
    }
  }, [modalOpen, editing, form]);

  // 切换邮箱模板时自动填充 SMTP 参数
  function handleEmailProviderChange(key: string) {
    const p = EMAIL_PRESETS[key] ?? EMAIL_PRESETS.custom;
    form.setFieldsValue({
      smtp_host: p.host,
      smtp_port: p.port,
      use_tls: p.useTls,
    });
  }

  // 邮箱地址变化时，收件人为空则自动跟随
  function handleEmailAccountChange(v: string) {
    const to = form.getFieldValue('to');
    if (!to) form.setFieldValue('to', v);
  }

  async function save() {
    const values = await form.validateFields();
    // 邮箱：快捷模板参数映射 + 发件人 = 邮箱地址
    const body: Record<string, unknown> = { ...values };
    if (values.type === 'email') {
      const p = EMAIL_PRESETS[values.email_provider ?? 'qq'] ?? EMAIL_PRESETS.qq;
      if (values.email_provider !== 'custom') {
        body.smtp_host = p.host;
        body.smtp_port = p.port;
        body.use_tls = p.useTls;
      }
      body.from = values.smtp_username;
    }
    delete body.email_provider;
    try {
      if (editing) {
        await api.updateChannel(editing.id, body);
        message.success('已保存');
      } else {
        await api.createChannel(body);
        message.success('已创建');
      }
      setModalOpen(false);
      load();
    } catch (e) {
      message.error(`保存失败: ${(e as Error).message}`);
    }
  }

  async function test(c: NotificationChannel) {
    setTesting(c.id);
    try {
      await api.testChannel(c.id);
      message.success('测试消息已发送');
    } catch (e) {
      message.error(`发送失败: ${(e as Error).message}`);
    } finally {
      setTesting(null);
    }
  }

  async function remove(c: NotificationChannel) {
    try {
      await api.deleteChannel(c.id);
      message.success('已删除');
      load();
    } catch (e) {
      message.error(`删除失败: ${(e as Error).message}`);
    }
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16, paddingRight: 40 }}>
        <Typography.Title level={4} style={{ margin: 0 }}>
          通知渠道
        </Typography.Title>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => {
            setEditing(null);
            setModalOpen(true);
          }}
        >
          新增渠道
        </Button>
      </div>

      <Table
        rowKey="id"
        loading={loading}
        dataSource={channels}
        pagination={false}
        columns={[
          {
            title: '名称',
            dataIndex: 'name',
            width: 220,
            render: (v, r) => (
              <Space>
                <Typography.Text strong>{v}</Typography.Text>
                <Tag color={channelTypeColor(r.type)}>{channelTypeLabel(r.type)}</Tag>
              </Space>
            ),
          },
          {
            title: '配置',
            ellipsis: true,
            render: (_, r) => (
              <Typography.Text type="secondary">
                {r.type === 'dingtalk' || r.type === 'feishu' || r.type === 'wecom'
                  ? r.webhook
                  : r.type === 'telegram'
                    ? `Bot: ${r.bot_token?.slice(0, 8)}... · Chat: ${r.chat_id}`
                    : r.type === 'ntfy'
                      ? `${r.server || 'https://ntfy.sh'}/${r.topic}`
                    : r.type === 'bark'
                      ? `${r.server || 'https://api.day.app'}/${r.device_key?.slice(0, 8)}...`
                      : r.type === 'gotify'
                        ? `${r.server}${r.secret_set ? '（已设置令牌）' : ''}`
                        : r.type === 'email'
                        ? `${r.smtp_username || r.from} → ${r.to}（${r.smtp_host}:${r.smtp_port}）`
                        : r.url}
                {r.secret_set ? '（已设置 Secret）' : ''}
              </Typography.Text>
            ),
          },
          {
            title: '启用',
            dataIndex: 'enabled',
            width: 80,
            render: (v: boolean, r) => (
              <Switch
                checked={v}
                onChange={async (checked) => {
                  await api.updateChannel(r.id, {
                    name: r.name,
                    type: r.type,
                    webhook: r.webhook,
                    url: r.url,
                    method: r.method,
                    bot_token: r.bot_token,
                    chat_id: r.chat_id,
                    topic: r.topic,
                    server: r.server,
                    device_key: r.device_key,
                    smtp_host: r.smtp_host,
                    smtp_port: r.smtp_port,
                    smtp_username: r.smtp_username,
                    from: r.from,
                    to: r.to,
                    enabled: checked,
                  });
                  load();
                }}
              />
            ),
          },
          {
            title: '操作',
            width: 220,
            render: (_, r) => (
              <Space>
                <Button
                  size="small"
                  icon={<SendOutlined />}
                  loading={testing === r.id}
                  onClick={() => test(r)}
                >
                  测试发送
                </Button>
                <Button
                  size="small"
                  icon={<EditOutlined />}
                  onClick={() => {
                    setEditing(r);
                    setModalOpen(true);
                  }}
                />
                <Popconfirm title="确认删除该渠道？" onConfirm={() => remove(r)} disabled={r.enabled}>
                  <Button size="small" danger icon={<DeleteOutlined />} title="删除" disabled={r.enabled} />
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />

      <Modal
        title={editing ? `编辑渠道 - ${editing.name}` : '新增通知渠道'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={save}
        okText="保存"
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="AI 任务通知" />
          </Form.Item>
          <Form.Item name="type" label="类型" rules={[{ required: true }]}>
            <Select
              disabled={!!editing}
              options={CHANNEL_TYPES.map((t) => ({ value: t.value, label: t.label }))}
            />
          </Form.Item>

          {(channelType === 'dingtalk' || channelType === 'feishu') && (
            <>
              <ChannelHelpLink type={channelType} onOpen={setHelpType} />
              <Form.Item name="webhook" label="Webhook" rules={[{ required: true }]}>
                <Input placeholder="https://oapi.dingtalk.com/robot/send?access_token=..." />
              </Form.Item>
              <Form.Item name="secret" label="Secret（加签，可选）">
                <Input.Password
                  placeholder={editing?.secret_set ? '已设置，留空保持不变' : 'SEC...'}
                  autoComplete="new-password"
                />
              </Form.Item>
            </>
          )}

          {channelType === 'wecom' && (
            <>
              <ChannelHelpLink type="wecom" onOpen={setHelpType} />
              <Form.Item name="webhook" label="Webhook" rules={[{ required: true }]}>
                <Input placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..." />
              </Form.Item>
            </>
          )}

          {channelType === 'telegram' && (
            <>
              <ChannelHelpLink type="telegram" onOpen={setHelpType} />
              <Form.Item name="bot_token" label="Bot Token" rules={[{ required: true }]}>
                <Input.Password
                  placeholder={editing?.bot_token ? '已设置，留空保持不变' : '123456:ABC-DEF...'}
                  autoComplete="new-password"
                />
              </Form.Item>
              <Form.Item name="chat_id" label="Chat ID" rules={[{ required: true }]}>
                <Input placeholder="-1001234567890（群组）或 @channelname（频道）" />
              </Form.Item>
            </>
          )}

          {channelType === 'ntfy' && (
            <>
              <ChannelHelpLink type="ntfy" onOpen={setHelpType} />
              <Form.Item
                name="topic"
                label="主题（Topic）"
                rules={[{ required: true, message: '请输入主题名' }]}
                help="手机 ntfy App 中订阅相同的主题名即可收到推送；主题名即密码，建议随机"
              >
                <Input
                  placeholder="ai-keeper-x7k2m9p4"
                  suffix={
                    <a
                      onClick={() => form.setFieldValue('topic', randomTopic())}
                      style={{ fontSize: 12 }}
                    >
                      随机
                    </a>
                  }
                />
              </Form.Item>
              <Form.Item
                name="server"
                label="服务器地址（可选）"
                help="留空使用官方免费服务 https://ntfy.sh；自建服务器填完整地址"
              >
                <Input placeholder="https://ntfy.sh" />
              </Form.Item>
            </>
          )}

          {channelType === 'bark' && (
            <>
              <ChannelHelpLink type="bark" onOpen={setHelpType} />
              <Form.Item
                name="device_key"
                label="设备 Key"
                rules={[{ required: true, message: '请输入设备 Key' }]}
                help="打开 Bark App，复制首页 URL 中 https://api.day.app/ 后面的一串字符"
              >
                <Input placeholder="abcdEFGH1234" />
              </Form.Item>
              <Form.Item
                name="server"
                label="服务器地址（可选）"
                help="留空使用官方服务 https://api.day.app；自建服务器填完整地址"
              >
                <Input placeholder="https://api.day.app" />
              </Form.Item>
            </>
          )}

          {channelType === 'gotify' && (
            <>
              <ChannelHelpLink type="gotify" onOpen={setHelpType} />
              <Form.Item
                name="server"
                label="服务器地址"
                rules={[{ required: true, message: '请输入 Gotify 服务器地址' }]}
                help="自建服务地址，如 http://gotify.example.com（Gotify 无官方公共服务）"
              >
                <Input placeholder="http://gotify.example.com" />
              </Form.Item>
              <Form.Item
                name="app_token"
                label="应用令牌 (App Token)"
                rules={editing ? [] : [{ required: true, message: '请输入应用令牌' }]}
                help="Gotify Web 端「APPS」创建应用后生成的 Token"
              >
                <Input.Password
                  placeholder={editing?.secret_set ? '已设置，留空保持不变' : 'Axxxxxxxxxx'}
                  autoComplete="new-password"
                />
              </Form.Item>
            </>
          )}

          {channelType === 'email' && (
            <>
              <ChannelHelpLink type="email" onOpen={setHelpType} />
              <Form.Item
                name="email_provider"
                label="邮箱类型"
                rules={[{ required: true }]}
                help="选择后自动填充 SMTP 服务器，只需填邮箱地址和授权码"
              >
                <Select
                  onChange={handleEmailProviderChange}
                  options={Object.entries(EMAIL_PRESETS).map(([value, p]) => ({
                    value,
                    label: value === 'custom' ? '自定义 SMTP（企业邮箱 / 自建）' : p.label,
                  }))}
                />
              </Form.Item>

              {emailProvider === 'custom' && (
                <>
                  <Form.Item
                    name="smtp_host"
                    label="SMTP 服务器"
                    rules={[{ required: true, message: '请输入 SMTP 服务器' }]}
                  >
                    <Input placeholder="smtp.example.com" />
                  </Form.Item>
                  <Form.Item
                    name="smtp_port"
                    label="端口"
                    rules={[{ required: true }]}
                    help="465 = SSL；587 = STARTTLS"
                  >
                    <InputNumber min={1} max={65535} style={{ width: '100%' }} placeholder="465" />
                  </Form.Item>
                </>
              )}

              <Form.Item
                name="smtp_username"
                label="邮箱地址"
                rules={[
                  { required: true, message: '请输入邮箱地址' },
                  { type: 'email', message: '邮箱格式不正确' },
                ]}
                extra={emailProvider !== 'custom' ? `SMTP：${preset.host}:${preset.port}（自动填充）` : undefined}
              >
                <Input
                  placeholder={emailProvider === 'gmail' ? 'me@gmail.com' : 'me@qq.com'}
                  autoComplete="off"
                  onChange={(e) => handleEmailAccountChange(e.target.value)}
                />
              </Form.Item>
              <Form.Item
                name="smtp_password"
                label={preset.passwordLabel}
                rules={editing ? [] : [{ required: true, message: '请填写密码' }]}
                help={preset.passwordHint}
              >
                <Input.Password
                  placeholder={editing ? '留空保持不变' : undefined}
                  autoComplete="new-password"
                />
              </Form.Item>
              <Form.Item
                name="to"
                label="收件人"
                rules={[
                  { required: true, message: '请输入收件人邮箱' },
                  {
                    validator: (_, v: string) => {
                      if (!v) return Promise.resolve();
                      const bad = v.split(',').map((s) => s.trim()).filter((s) => s && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(s));
                      return bad.length ? Promise.reject(new Error('存在格式不正确的邮箱地址')) : Promise.resolve();
                    },
                  },
                ]}
                help="通知将发送到该邮箱，多个收件人用英文逗号分隔"
              >
                <Input placeholder="me@example.com" />
              </Form.Item>
            </>
          )}

          {channelType === 'webhook' && (
            <>
              <Form.Item name="url" label="URL" rules={[{ required: true }]}>
                <Input placeholder="https://example.com/notify" />
              </Form.Item>
              <Form.Item name="method" label="请求方法">
                <Input placeholder="POST" />
              </Form.Item>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                通知将以 JSON POST：{'{ event, title, content, task, at }'}
              </Typography.Text>
            </>
          )}

          <Form.Item name="enabled" label="启用" valuePropName="checked" style={{ marginTop: 16 }}>
            <Switch />
          </Form.Item>
        </Form>
      </Modal>

      {/* 配置指引侧边抽屉 */}
      <Drawer
        open={!!helpType}
        onClose={() => setHelpType(null)}
        title={helpType ? `${channelTypeLabel(helpType)}配置指引` : ''}
        width={440}
        zIndex={1200}
      >
        {helpType ? CHANNEL_HELP[helpType] : null}
      </Drawer>
    </div>
  );
}
