import { useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Drawer,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Radio,
  Select,
  Space,
  Typography,
  Upload,
  message,
} from 'antd';
import {
  CloudUploadOutlined,
  DeleteOutlined,
  DownloadOutlined,
  QuestionCircleOutlined,
  SafetyCertificateOutlined,
  UploadOutlined,
} from '@ant-design/icons';
import { api, downloadConfigBackup } from '../../api';
import type { ProxyConfig } from '../../types';
import { fmtDateTime } from '../../utils';

const { Text } = Typography;

const WEBDAV_HELP = (
  <div>
    <Typography.Title level={5} style={{ marginBottom: 8 }}>坚果云 WebDAV 配置指引</Typography.Title>
    <Typography.Paragraph style={{ fontSize: 13, marginBottom: 8 }}>
      坚果云提供免费 WebDAV 接口，选择「坚果云」模板后服务器地址自动填好，你只需要：
    </Typography.Paragraph>
    <Typography.Paragraph style={{ fontSize: 13, marginBottom: 8 }}>
      <strong>1. 生成应用密码（必需）：</strong>
      <ol style={{ paddingLeft: 20, margin: '4px 0' }}>
        <li>登录坚果云网页版 → 右上角头像 →「账户信息」</li>
        <li>「安全选项」→「第三方应用管理」→「添加应用密码」</li>
        <li>名称随意（如「AI 订阅管家」），生成后复制密码</li>
      </ol>
    </Typography.Paragraph>
    <Typography.Paragraph style={{ fontSize: 13, marginBottom: 8 }}>
      <strong>2. 填写表单：</strong>
      <ul style={{ paddingLeft: 20, margin: '4px 0' }}>
        <li>服务器：<Typography.Text code>https://dav.jianguoyun.com/dav/</Typography.Text>（模板已填）</li>
        <li>用户名：坚果云登录邮箱</li>
        <li>密码：<strong>应用密码</strong>（⚠️ 不是登录密码，直接用登录密码会 401）</li>
        <li>远程目录：默认 <Typography.Text code>/keeper/</Typography.Text>，备份文件存放在该目录</li>
      </ul>
    </Typography.Paragraph>
    <Typography.Paragraph style={{ fontSize: 13, marginBottom: 8 }}>
      保存后先「测试连接」确认可用，再「立即备份」。<strong>从 WebDAV 恢复</strong>会把云端备份合并导入本机（已存在的条目跳过，不会覆盖）。
    </Typography.Paragraph>
    <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
      自建 WebDAV（Alist / Nextcloud / 群晖等）选「自定义」模板，填写对应服务器地址和账号密码。
    </Typography.Paragraph>
  </div>
);

function importSummary(res: Record<string, number>): string {
  const parts: string[] = [];
  if (res.services) parts.push(`模型服务 ${res.services}`);
  if (res.channels) parts.push(`通知渠道 ${res.channels}`);
  if (res.tasks) parts.push(`定时任务 ${res.tasks}`);
  if (res.proxy) parts.push('网络代理');
  return parts.length ? `导入完成：${parts.join('、')}` : '导入完成：无新增（已存在的条目已跳过）';
}

export default function SettingsPage() {
  const [form] = Form.useForm<ProxyConfig>();
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<string | null>(null);
  const [cleanupDays, setCleanupDays] = useState(30);
  const [cleaning, setCleaning] = useState(false);
  const [version, setVersion] = useState('');

  // 配置导入/导出
  const [exporting, setExporting] = useState(false);
  const [importing, setImporting] = useState(false);

  // WebDAV
  const [webdavForm] = Form.useForm();
  const [webdavProvider, setWebdavProvider] = useState<string>('jianguoyun');
  const [passwordSet, setPasswordSet] = useState(false);
  const [lastSync, setLastSync] = useState('');
  const [testingWebdav, setTestingWebdav] = useState(false);
  const [webdavTestResult, setWebdavTestResult] = useState<string | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [restoring, setRestoring] = useState(false);
  const [savingWebdav, setSavingWebdav] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);

  // 版本更新
  const [updateInfo, setUpdateInfo] = useState<{
    current_version: string;
    latest_version: string;
    update_available: boolean;
    release_url: string;
    notes?: string;
  } | null>(null);
  const [checking, setChecking] = useState(false);
  const [upgrading, setUpgrading] = useState(false);

  useEffect(() => {
    api
      .version()
      .then((v) => setVersion(v.version))
      .catch(() => {});
  }, []);

  useEffect(() => {
    api
      .getProxy()
      .then((cfg) => form.setFieldsValue(cfg))
      .catch((e) => message.error(`加载设置失败: ${(e as Error).message}`));
  }, [form]);

  useEffect(() => {
    api
      .getWebdav()
      .then((cfg) => {
        setPasswordSet(cfg.password_set);
        setLastSync(cfg.last_sync || '');
        const isJGY = (cfg.server || '').includes('jianguoyun');
        setWebdavProvider(isJGY ? 'jianguoyun' : cfg.server ? 'custom' : 'jianguoyun');
        webdavForm.setFieldsValue({
          server: cfg.server || 'https://dav.jianguoyun.com/dav/',
          username: cfg.username,
          password: '',
          path: cfg.path || '/keeper/',
        });
      })
      .catch(() => {});
  }, [webdavForm]);

  async function save() {
    const values = await form.validateFields();
    setLoading(true);
    try {
      await api.putProxy(values);
      message.success('已保存，全局代理已生效');
    } catch (e) {
      message.error(`保存失败: ${(e as Error).message}`);
    } finally {
      setLoading(false);
    }
  }

  async function test() {
    const values = await form.validateFields();
    setTesting(true);
    setTestResult(null);
    try {
      const res = await api.testProxy(values);
      if (res.ok) {
        setTestResult(`连接成功（${res.latency_ms} ms）`);
      } else {
        setTestResult(`连接失败：${res.error ?? `HTTP ${res.status}`}`);
      }
    } catch (e) {
      setTestResult(`连接失败：${(e as Error).message}`);
    } finally {
      setTesting(false);
    }
  }

  async function handleExport() {
    setExporting(true);
    try {
      await downloadConfigBackup();
      message.success('配置已导出');
    } catch (e) {
      message.error(`导出失败: ${(e as Error).message}`);
    } finally {
      setExporting(false);
    }
  }

  async function handleImportFile(file: File) {
    setImporting(true);
    try {
      const text = await file.text();
      const res = await api.importConfig(text);
      message.success(importSummary(res));
    } catch (e) {
      message.error(`导入失败: ${(e as Error).message}`);
    } finally {
      setImporting(false);
    }
  }

  function handleWebdavProviderChange(value: string) {
    setWebdavProvider(value);
    if (value === 'jianguoyun') {
      webdavForm.setFieldsValue({ server: 'https://dav.jianguoyun.com/dav/' });
    }
  }

  async function saveWebdav() {
    const values = await webdavForm.validateFields();
    setSavingWebdav(true);
    try {
      await api.putWebdav(values);
      if (values.password) setPasswordSet(true);
      message.success('WebDAV 配置已保存');
    } catch (e) {
      message.error(`保存失败: ${(e as Error).message}`);
    } finally {
      setSavingWebdav(false);
    }
  }

  async function testWebdav() {
    setTestingWebdav(true);
    setWebdavTestResult(null);
    try {
      const res = await api.testWebdav();
      setWebdavTestResult(
        res.ok ? `连接成功（${res.latency_ms} ms）` : `连接失败：${res.error ?? '未知错误'}`,
      );
    } catch (e) {
      setWebdavTestResult(`连接失败：${(e as Error).message}`);
    } finally {
      setTestingWebdav(false);
    }
  }

  async function syncNow() {
    setSyncing(true);
    try {
      const res = await api.webdavSync();
      setLastSync(res.synced_at);
      message.success(`已备份到 WebDAV（${(res.size / 1024).toFixed(1)} KB）`);
    } catch (e) {
      message.error(`备份失败: ${(e as Error).message}`);
    } finally {
      setSyncing(false);
    }
  }

  async function restoreNow() {
    setRestoring(true);
    try {
      const res = await api.webdavRestore();
      message.success(importSummary(res));
    } catch (e) {
      message.error(`恢复失败: ${(e as Error).message}`);
    } finally {
      setRestoring(false);
    }
  }

  async function checkUpdate() {
    setChecking(true);
    setUpdateInfo(null);
    try {
      setUpdateInfo(await api.checkUpdate());
    } catch (e) {
      message.error(`检查更新失败: ${(e as Error).message}`);
    } finally {
      setChecking(false);
    }
  }

  async function upgradeNow() {
    setUpgrading(true);
    const prevVersion = version;
    try {
      await api.upgradeVersion();
      message.success('新版本已就位，服务正在重启…');
      // 轮询 /api/version，服务恢复且版本变化后自动刷新页面加载新前端
      const deadline = Date.now() + 90_000;
      const poll = window.setInterval(async () => {
        try {
          const v = await api.version();
          if (v.version !== prevVersion) {
            window.clearInterval(poll);
            message.success(`已升级到 v${v.version.replace(/^v/, '')}，页面即将刷新`);
            setTimeout(() => window.location.reload(), 1200);
          }
        } catch {
          /* 服务重启中，继续等待 */
        }
        if (Date.now() > deadline) {
          window.clearInterval(poll);
          message.warning('升级仍在进行，请稍后手动刷新页面');
          setUpgrading(false);
        }
      }, 2000);
    } catch (e) {
      message.error(`升级失败: ${(e as Error).message}`);
      setUpgrading(false);
    }
  }

  async function cleanupLogs() {
    setCleaning(true);
    try {
      const res = await api.cleanupLogs(cleanupDays);
      message.success(`已清理 ${res.deleted_executions} 条执行记录、${res.deleted_test_logs} 条测试日志`);
    } catch (e) {
      message.error(`清理失败: ${(e as Error).message}`);
    } finally {
      setCleaning(false);
    }
  }

  return (
    <div style={{ width: '100%' }}>
      <Typography.Title level={4} style={{ marginBottom: 12, paddingRight: 40 }}>
        系统设置
      </Typography.Title>
      <Card title="全局网络代理" size="small">
        <Typography.Paragraph type="secondary" style={{ fontSize: 13 }}>
          代理对全部 AI 供应商请求生效（获取模型、查询额度、模型测试、模型预热）。
          支持 http://、https://、socks5:// 协议。
        </Typography.Paragraph>
        <Form form={form} layout="vertical">
          <Form.Item name="mode">
            <Radio.Group
              options={[
                { value: 'none', label: '不使用代理' },
                { value: 'proxy', label: '使用代理' },
              ]}
            />
          </Form.Item>
          <Form.Item name="url" label="代理地址">
            <Input placeholder="http://127.0.0.1:7890" />
          </Form.Item>
          <Space>
            <Button
              icon={<SafetyCertificateOutlined />}
              loading={testing}
              onClick={test}
            >
              测试连接
            </Button>
            <Button type="primary" loading={loading} onClick={save}>
              保存
            </Button>
          </Space>
          {testResult && (
            <Alert
              style={{ marginTop: 16 }}
              type={testResult.startsWith('连接成功') ? 'success' : 'error'}
              message={testResult}
              showIcon
            />
          )}
        </Form>
      </Card>

      <Card title="配置导入 / 导出" size="small" style={{ marginTop: 16 }}>
        <Typography.Paragraph type="secondary" style={{ fontSize: 13 }}>
          导出内容包含模型服务、定时任务、通知渠道和网络代理的完整配置。
          导入按 ID 合并，已存在的条目自动跳过，不会覆盖本机数据。
        </Typography.Paragraph>
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
          message="导出文件中的凭证为明文（便于跨设备迁移），请妥善保管，不要分享给他人。"
        />
        <Space>
          <Button icon={<DownloadOutlined />} loading={exporting} onClick={handleExport}>
            导出配置
          </Button>
          <Upload
            accept=".json"
            showUploadList={false}
            beforeUpload={(file) => {
              handleImportFile(file);
              return false;
            }}
          >
            <Button icon={<UploadOutlined />} loading={importing}>
              导入配置
            </Button>
          </Upload>
        </Space>
      </Card>

      <Card title="WebDAV 同步" size="small" style={{ marginTop: 16 }}>
        <Space style={{ marginBottom: 12 }}>
          <Typography.Link style={{ fontSize: 13 }} onClick={() => setHelpOpen(true)}>
            <QuestionCircleOutlined /> 查看 WebDAV 配置指引
          </Typography.Link>
          {lastSync && (
            <Text type="secondary" style={{ fontSize: 12 }}>
              上次备份：{fmtDateTime(lastSync)}
            </Text>
          )}
        </Space>
        <Form form={webdavForm} layout="vertical">
          <Form.Item label="服务商模板">
            <Select
              value={webdavProvider}
              onChange={handleWebdavProviderChange}
              options={[
                { value: 'jianguoyun', label: '坚果云' },
                { value: 'custom', label: '自定义（Alist / Nextcloud / 群晖等）' },
              ]}
            />
          </Form.Item>
          <Form.Item
            name="server"
            label="服务器地址"
            rules={[{ required: true, message: '请输入服务器地址' }]}
          >
            <Input placeholder="https://dav.jianguoyun.com/dav/" />
          </Form.Item>
          <Form.Item name="username" label="用户名">
            <Input placeholder="坚果云登录邮箱 / WebDAV 账号" autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="password"
            label="密码 / 应用密码"
            help={
              passwordSet
                ? '已设置，留空保持不变'
                : webdavProvider === 'jianguoyun'
                  ? '坚果云需使用「应用密码」，不是登录密码（见配置指引）'
                  : 'WebDAV 账户密码'
            }
          >
            <Input.Password
              placeholder={passwordSet ? '留空保持不变' : '应用密码 / 账户密码'}
              autoComplete="new-password"
            />
          </Form.Item>
          <Form.Item name="path" label="远程目录" help="备份文件存放目录，目录不存在会自动创建">
            <Input placeholder="/keeper/" />
          </Form.Item>
          <Space wrap>
            <Button type="primary" loading={savingWebdav} onClick={saveWebdav}>
              保存配置
            </Button>
            <Button loading={testingWebdav} onClick={testWebdav}>
              测试连接
            </Button>
            <Popconfirm title="立即备份当前配置到 WebDAV？" onConfirm={syncNow}>
              <Button icon={<CloudUploadOutlined />} loading={syncing}>
                立即备份
              </Button>
            </Popconfirm>
            <Popconfirm
              title="从 WebDAV 恢复配置？已存在的条目会跳过。"
              onConfirm={restoreNow}
            >
              <Button loading={restoring}>从 WebDAV 恢复</Button>
            </Popconfirm>
          </Space>
          {webdavTestResult && (
            <Alert
              style={{ marginTop: 12 }}
              type={webdavTestResult.startsWith('连接成功') ? 'success' : 'error'}
              message={webdavTestResult}
              showIcon
            />
          )}
        </Form>
      </Card>

      <Card title="版本更新" size="small" style={{ marginTop: 16 }}>
        <Typography.Paragraph type="secondary" style={{ fontSize: 13 }}>
          检查 GitHub Release 新版本，下载后自动替换程序并重启（前端内嵌于程序，重启后页面自动刷新）。
          Docker 部署请改为拉取新镜像。
        </Typography.Paragraph>
        <Space wrap>
          <Button loading={checking} onClick={checkUpdate}>
            检查更新
          </Button>
          {updateInfo?.update_available && (
            <Popconfirm
              title={`升级到 ${updateInfo.latest_version}？服务将自动重启。`}
              onConfirm={upgradeNow}
            >
              <Button type="primary" loading={upgrading}>
                一键升级到 {updateInfo.latest_version}
              </Button>
            </Popconfirm>
          )}
          {updateInfo && !updateInfo.update_available && (
            <Text type="secondary">
              {updateInfo.current_version === 'dev'
                ? `当前为开发版本，最新 Release：${updateInfo.latest_version}`
                : `已是最新版本 v${updateInfo.current_version}`}
            </Text>
          )}
          {updateInfo?.release_url && (
            <Typography.Link
              style={{ fontSize: 12 }}
              href={updateInfo.release_url}
              target="_blank"
            >
              查看Release说明
            </Typography.Link>
          )}
        </Space>
        {updateInfo?.update_available && updateInfo.notes && (
          <pre
            style={{
              marginTop: 12,
              background: 'rgba(255,255,255,0.06)',
              padding: 10,
              borderRadius: 6,
              fontSize: 12,
              maxHeight: 160,
              overflow: 'auto',
              whiteSpace: 'pre-wrap',
            }}
          >
            {updateInfo.notes}
          </pre>
        )}
      </Card>

      <Card title="日志清理" size="small" style={{ marginTop: 16 }}>
        <Typography.Paragraph type="secondary" style={{ fontSize: 13 }}>
          清理过期的定时任务执行记录和模型测试日志，释放存储空间。
        </Typography.Paragraph>
        <Space>
          <span>保留最近</span>
          <InputNumber
            min={1}
            max={365}
            value={cleanupDays}
            onChange={(v) => setCleanupDays(v ?? 30)}
            style={{ width: 80 }}
          />
          <span>天的日志</span>
          <Popconfirm
            title={`确认清理 ${cleanupDays} 天前的所有日志？`}
            onConfirm={async () => {
              setCleaning(true);
              try {
                const res = await api.cleanupLogs(cleanupDays);
                message.success(
                  `已清理 ${res.deleted_executions} 条执行记录、${res.deleted_test_logs} 条测试日志`,
                );
              } catch (e) {
                message.error(`清理失败: ${(e as Error).message}`);
              } finally {
                setCleaning(false);
              }
            }}
          >
            <Button danger icon={<DeleteOutlined />} loading={cleaning}>
              立即清理
            </Button>
          </Popconfirm>
        </Space>
      </Card>

      <Typography.Text
        type="secondary"
        style={{ display: 'block', textAlign: 'center', marginTop: 16, fontSize: 12 }}
      >
        AI 订阅管家 {version ? `v${version.replace(/^v/, '')}` : ''} · 数据本地存储
      </Typography.Text>

      <Drawer
        open={helpOpen}
        onClose={() => setHelpOpen(false)}
        title="WebDAV 配置指引"
        width={440}
        zIndex={1200}
      >
        {WEBDAV_HELP}
      </Drawer>
    </div>
  );
}
