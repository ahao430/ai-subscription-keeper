import { useCallback, useEffect, useState } from 'react';
import {
  Button,
  Popconfirm,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  CloudDownloadOutlined,
  DeleteOutlined,
  EditOutlined,
  HistoryOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import { api } from '../../api';
import type { ModelService, ProviderTypeInfo } from '../../types';
import { timeAgo } from '../../utils';
import ServiceModal from './ServiceModal';
import TestLogDrawer from './TestLogDrawer';

const { Text } = Typography;

export default function ModelServicesPage() {
  const [services, setServices] = useState<ModelService[]>([]);
  const [providers, setProviders] = useState<ProviderTypeInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<ModelService | null>(null);
  const [logService, setLogService] = useState<ModelService | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [svcs, pts] = await Promise.all([api.listServices(), api.providerTypes()]);
      setServices(svcs);
      setProviders(pts);
    } catch (e) {
      message.error(`加载失败: ${(e as Error).message}`);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function toggleEnabled(svc: ModelService, enabled: boolean) {
    try {
      await api.updateService(svc.id, {
        name: svc.name,
        provider_type: svc.provider_type,
        default_warmup_model: svc.default_warmup_model,
        default_test_model: svc.default_test_model,
        default_test_prompt: svc.default_test_prompt,
        quota_labels: svc.quota_labels,
        enabled,
        sort_order: svc.sort_order,
      });
      setServices((prev) => prev.map((s) => (s.id === svc.id ? { ...s, enabled } : s)));
    } catch (e) {
      message.error(`操作失败: ${(e as Error).message}`);
    }
  }

  async function refreshModels(svc: ModelService) {
    try {
      const resp = await api.refreshModels(svc.id);
      setServices((prev) =>
        prev.map((s) => (s.id === svc.id ? { ...s, models: resp.models } : s)),
      );
      message.success(`已获取 ${resp.models.length} 个模型`);
    } catch (e) {
      message.error(`获取模型失败: ${(e as Error).message}`);
    }
  }

  async function remove(svc: ModelService) {
    try {
      await api.deleteService(svc.id);
      message.success('已删除');
      load();
    } catch (e) {
      message.error(`删除失败: ${(e as Error).message}`);
    }
  }

  const providerName = (code: string) =>
    providers.find((p) => p.code === code)?.name ?? code;

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16, paddingRight: 40 }}>
        <Typography.Title level={4} style={{ margin: 0 }}>
          模型服务
        </Typography.Title>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => {
            setEditing(null);
            setModalOpen(true);
          }}
        >
          新增模型服务
        </Button>
      </div>

      <Table
        rowKey="id"
        loading={loading}
        dataSource={services}
        pagination={false}
        scroll={{ x: 1110 }}
        columns={[
          {
            title: '名称',
            dataIndex: 'name',
            width: 260,
            ellipsis: true,
            render: (v, r) => (
              <Space>
                <Text strong>{v}</Text>
                <Tag>{providerName(r.provider_type)}</Tag>
              </Space>
            ),
          },
          {
            title: '模型',
            dataIndex: 'models',
            width: 150,
            render: (models: string[]) =>
              models.length ? <TooltipHost models={models} /> : <Text type="secondary">未获取</Text>,
          },
          {
            title: '默认预热模型',
            dataIndex: 'default_warmup_model',
            width: 150,
            render: (v: string) => v || <Text type="secondary">-</Text>,
          },
          {
            title: '默认测试模型',
            dataIndex: 'default_test_model',
            width: 150,
            render: (v: string) => v || <Text type="secondary">-</Text>,
          },
          {
            title: '最近额度查询',
            dataIndex: 'last_quota_at',
            width: 120,
            render: (v: string, r) =>
              r.last_quota_error ? (
                <Text type="danger" title={r.last_quota_error}>
                  查询失败
                </Text>
              ) : (
                timeAgo(v)
              ),
          },
          {
            title: '启用',
            dataIndex: 'enabled',
            width: 80,
            render: (v: boolean, r) => (
              <Switch checked={v} onChange={(checked) => toggleEnabled(r, checked)} />
            ),
          },
          {
            title: '操作',
            width: 200,
            render: (_, r) => (
              <Space size={4}>
                <Button
                  size="small"
                  icon={<CloudDownloadOutlined />}
                  onClick={() => refreshModels(r)}
                  title="重新获取模型列表"
                />
                <Button
                  size="small"
                  icon={<EditOutlined />}
                  onClick={() => {
                    setEditing(r);
                    setModalOpen(true);
                  }}
                  title="编辑"
                />
                <Button
                  size="small"
                  icon={<HistoryOutlined />}
                  onClick={() => setLogService(r)}
                  title="测试日志"
                />
                <Popconfirm title="确认删除该模型服务？" onConfirm={() => remove(r)} disabled={r.enabled}>
                  <Button size="small" danger icon={<DeleteOutlined />} title="删除" disabled={r.enabled} />
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />

      <ServiceModal
        open={modalOpen}
        providers={providers}
        editing={editing}
        onClose={() => setModalOpen(false)}
        onSaved={load}
      />
      <TestLogDrawer
        open={!!logService}
        service={logService}
        onClose={() => setLogService(null)}
      />
    </div>
  );
}

function TooltipHost({ models }: { models: string[] }) {
  return (
    <Text ellipsis style={{ maxWidth: 200 }} title={models.join('、')}>
      {models.length} 个模型
    </Text>
  );
}
