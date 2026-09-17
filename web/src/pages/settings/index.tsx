import { useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Radio,
  Space,
  Typography,
  message,
} from 'antd';
import { DeleteOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { api } from '../../api';
import type { ProxyConfig } from '../../types';

export default function SettingsPage() {
  const [form] = Form.useForm<ProxyConfig>();
  const [loading, setLoading] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<string | null>(null);
  const [cleanupDays, setCleanupDays] = useState(30);
  const [cleaning, setCleaning] = useState(false);
  const [version, setVersion] = useState('');

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

      <Card title="日志清理" size="small" style={{ marginTop: 16 }}>
        <Typography.Paragraph type="secondary" style={{ fontSize: 13 }}>
          清理超过指定天数的定时任务执行记录和模型测试日志，释放存储空间。
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
    </div>
  );
}
