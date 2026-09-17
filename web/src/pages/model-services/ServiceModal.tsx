import { useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Col,
  Form,
  Input,
  Modal,
  Row,
  Select,
  Space,
  Switch,
  Tooltip,
  Typography,
  message,
} from 'antd';
import { ApiOutlined, CloudDownloadOutlined } from '@ant-design/icons';
import { api } from '../../api';
import type { ModelService, ProviderTypeInfo } from '../../types';

const { Text } = Typography;

export interface ServiceFormValues {
  name: string;
  provider_type: string;
  billing_type?: 'subscription' | 'pay_as_you_go';
  default_warmup_model?: string;
  default_test_model?: string;
  default_test_prompt?: string;
  enabled?: boolean;
}

export default function ServiceModal({
  open,
  providers,
  editing,
  onClose,
  onSaved,
}: {
  open: boolean;
  providers: ProviderTypeInfo[];
  editing: ModelService | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [form] = Form.useForm<ServiceFormValues>();
  const [credential, setCredential] = useState<Record<string, string>>({});
  const [models, setModels] = useState<string[]>([]);
  const [quotaLabels, setQuotaLabels] = useState<{ code: string; label: string }[]>([]);
  const [testing, setTesting] = useState(false);
  const [fetching, setFetching] = useState(false);
  const [saving, setSaving] = useState(false);

  const providerType = Form.useWatch('provider_type', form);
  const pt = providers.find((p) => p.code === providerType);

  useEffect(() => {
    if (open) {
      form.resetFields();
      setCredential({});
      setModels([]);
      setQuotaLabels([]);
      if (editing) {
        form.setFieldsValue({
          name: editing.name,
          provider_type: editing.provider_type,
          billing_type: editing.billing_type || 'subscription',
          default_warmup_model: editing.default_warmup_model || undefined,
          default_test_model: editing.default_test_model || undefined,
          default_test_prompt: editing.default_test_prompt || 'hi',
          enabled: editing.enabled,
        });
        setCredential(editing.credential ?? {});
        setModels(editing.models ?? []);
        const labels = Object.entries(editing.quota_labels ?? {}).map(([code, label]) => ({
          code,
          label,
        }));
        setQuotaLabels(labels.length ? labels : [{ code: '', label: '' }]);
      } else {
        form.setFieldsValue({
          provider_type: providers[0]?.code,
          billing_type: 'subscription',
          default_test_prompt: 'hi',
          enabled: true,
        });
        setQuotaLabels([{ code: '', label: '' }]);
      }
    }
  }, [open, editing, form]);

  async function handleFetchModels() {
    if (!pt) return;
    setFetching(true);
    try {
      const resp = await api.adhocModels(pt.code, credential);
      setModels(resp.models);
      message.success(`获取到 ${resp.models.length} 个模型`);
    } catch (e) {
      message.error(`获取模型失败: ${(e as Error).message}`);
    } finally {
      setFetching(false);
    }
  }

  async function handleTestConnection() {
    if (!pt) return;
    setTesting(true);
    try {
      await api.adhocModels(pt.code, credential);
      message.success('连接成功');
    } catch (e) {
      message.error(`连接失败: ${(e as Error).message}`);
    } finally {
      setTesting(false);
    }
  }

  async function handleSave() {
    const values = await form.validateFields();
    if (!pt) {
      message.error('请选择供应商');
      return;
    }
    // 必填凭证前端校验（新建时）
    if (!editing) {
      for (const f of pt.fields) {
        if (f.required && !credential[f.name]) {
          message.error(`请填写 ${f.label}`);
          return;
        }
      }
    }
    setSaving(true);
    try {
      const labels: Record<string, string> = {};
      for (const { code, label } of quotaLabels) {
        if (code && label) labels[code] = label;
      }
      const body = {
        ...values,
        credential,
        quota_labels: labels,
      };
      if (editing) {
        await api.updateService(editing.id, body);
        message.success('已保存');
      } else {
        await api.createService(body);
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

  const modelOptions = models.map((m) => ({ value: m, label: m }));

  return (
    <Modal
      title={editing ? `编辑模型服务 - ${editing.name}` : '新增模型服务'}
      open={open}
      onCancel={onClose}
      onOk={handleSave}
      confirmLoading={saving}
      okText="保存"
      width={640}
      destroyOnClose
    >
      <Form form={form} layout="vertical">
        <Row gutter={16}>
          <Col span={12}>
            <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
              <Input placeholder="智谱 Coding 主账号" />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item
              name="provider_type"
              label="供应商"
              rules={[{ required: true, message: '请选择供应商' }]}
            >
              <Select
                options={providers.map((p) => ({
                  value: p.code,
                  label: (
                    <Space>
                      {p.logo && (
                        <img
                          src={p.logo}
                          alt={p.name}
                          style={{ width: 16, height: 16, borderRadius: 3 }}
                        />
                      )}
                      {p.name}
                    </Space>
                  ),
                }))}
                disabled={!!editing}
                placeholder="选择供应商"
              />
            </Form.Item>
          </Col>
        </Row>

        <Form.Item name="billing_type" label="计费类型">
          <Select
            options={[
              { value: 'subscription', label: '订阅' },
              { value: 'pay_as_you_go', label: '按量计费' },
            ]}
          />
        </Form.Item>

        {pt && (
          <div style={{ marginBottom: 16 }}>
            {pt.auth_note && (
              <Alert type="info" showIcon message={pt.auth_note} style={{ marginBottom: 12 }} />
            )}
            {pt.fields.map((f) => (
              <Form.Item
                key={f.name}
                label={f.label}
                help={f.help}
                style={{ marginBottom: 12 }}
              >
                <Input
                  type={f.type === 'password' ? 'password' : 'text'}
                  placeholder={
                    f.placeholder ||
                    (f.type === 'password' && editing ? '留空保持不变' : undefined)
                  }
                  value={credential[f.name]}
                  onChange={(e) =>
                    setCredential((c) => ({ ...c, [f.name]: e.target.value }))
                  }
                  autoComplete="new-password"
                />
              </Form.Item>
            ))}
            <Space>
              <Button icon={<ApiOutlined />} loading={testing} onClick={handleTestConnection}>
                测试连接
              </Button>
              <Button
                icon={<CloudDownloadOutlined />}
                loading={fetching}
                onClick={handleFetchModels}
              >
                获取模型
              </Button>
              {models.length > 0 && (
                <Text type="secondary">已获取 {models.length} 个模型</Text>
              )}
            </Space>
          </div>
        )}

        <Row gutter={16}>
          <Col span={12}>
            <Form.Item name="default_warmup_model" label="默认预热模型">
              <Select
                showSearch
                allowClear
                placeholder="选择或输入模型"
                options={modelOptions}
              />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item name="default_test_model" label="默认测试模型">
              <Select
                showSearch
                allowClear
                placeholder="选择或输入模型"
                options={modelOptions}
              />
            </Form.Item>
          </Col>
        </Row>

        <Form.Item name="default_test_prompt" label="默认测试提示词">
          <Input placeholder="hi" />
        </Form.Item>

        <Form.Item label="额度展示配置" style={{ marginBottom: 8 }}>
          <Text type="secondary" style={{ fontSize: 12 }}>
            自定义各额度维度的展示名称（如 5h → 5小时限额），未配置时使用供应商默认文案
          </Text>
          {quotaLabels.map((item, idx) => (
            <Space.Compact key={idx} style={{ display: 'flex', marginBottom: 4 }}>
              <Input
                style={{ width: 140 }}
                placeholder="code（如 5h）"
                value={item.code}
                onChange={(e) => {
                  const next = [...quotaLabels];
                  next[idx] = { ...next[idx], code: e.target.value };
                  setQuotaLabels(next);
                }}
              />
              <Input
                style={{ width: 220 }}
                placeholder="展示名称（如 MCP服务周限额）"
                value={item.label}
                onChange={(e) => {
                  const next = [...quotaLabels];
                  next[idx] = { ...next[idx], label: e.target.value };
                  setQuotaLabels(next);
                }}
              />
            </Space.Compact>
          ))}
          <Button
            size="small"
            type="dashed"
            style={{ marginTop: 4 }}
            onClick={() => setQuotaLabels((l) => [...l, { code: '', label: '' }])}
          >
            + 添加额度文案
          </Button>
        </Form.Item>

        <Form.Item name="enabled" label="启用" valuePropName="checked">
          <Switch />
        </Form.Item>
      </Form>
    </Modal>
  );
}
