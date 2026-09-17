import { useCallback, useEffect, useState } from 'react';
import {
  Button,
  Drawer,
  Empty,
  List,
  Popconfirm,
  Space,
  Spin,
  Tag,
  Typography,
  message,
} from 'antd';
import { DeleteOutlined, ReloadOutlined } from '@ant-design/icons';
import { api } from '../../api';
import type { ModelService, TestLog } from '../../types';
import { fmtDateTime, fmtDuration } from '../../utils';

const { Text } = Typography;

function statusTag(status: string) {
  if (status === 'success') return <Tag color="success">成功</Tag>;
  if (status === 'failed') return <Tag color="error">失败</Tag>;
  return <Tag color="processing">进行中</Tag>;
}

export default function TestLogDrawer({
  open,
  service,
  onClose,
}: {
  open: boolean;
  service: ModelService | null;
  onClose: () => void;
}) {
  const [logs, setLogs] = useState<TestLog[]>([]);
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    if (!service) return;
    setLoading(true);
    try {
      setLogs(await api.testLogs(service.id));
    } catch (e) {
      message.error(`加载测试日志失败: ${(e as Error).message}`);
    } finally {
      setLoading(false);
    }
  }, [service]);

  useEffect(() => {
    if (open) load();
  }, [open, load]);

  async function clearLogs() {
    if (!service) return;
    try {
      await api.deleteTestLogs(service.id);
      message.success('已清空');
      setLogs([]);
    } catch (e) {
      message.error(`清空失败: ${(e as Error).message}`);
    }
  }

  return (
    <Drawer
      title={`测试日志 - ${service?.name ?? ''}`}
      open={open}
      onClose={onClose}
      width={620}
      zIndex={1100}
      extra={
        <Space>
          <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>
            刷新
          </Button>
          {logs.length > 0 && (
            <Popconfirm title="确认清空所有测试日志？" onConfirm={clearLogs}>
              <Button danger icon={<DeleteOutlined />}>
                清空日志
              </Button>
            </Popconfirm>
          )}
        </Space>
      }
    >
      {loading && logs.length === 0 ? (
        <div style={{ textAlign: 'center', padding: 60 }}>
          <Spin />
        </div>
      ) : logs.length === 0 ? (
        <Empty description="暂无测试日志" />
      ) : (
        <List
          dataSource={logs}
          renderItem={(log) => (
            <List.Item style={{ padding: '10px 4px', flexDirection: 'column', alignItems: 'stretch' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', width: '100%' }}>
                <Space size={12}>
                  {statusTag(log.status)}
                  <Text strong>{log.model}</Text>
                  <Text type="secondary">{fmtDateTime(log.started_at)}</Text>
                  <Text type="secondary">
                    耗时{' '}
                    {log.finished_at
                      ? fmtDuration(
                          new Date(log.finished_at).getTime() - new Date(log.started_at).getTime(),
                        )
                      : '-'}
                  </Text>
                </Space>
              </div>
              {log.prompt && (
                <Text type="secondary" style={{ fontSize: 12, marginTop: 4 }}>
                  提示词：{log.prompt}
                </Text>
              )}
              {log.error && (
                <Text type="danger" style={{ fontSize: 12, marginTop: 4, display: 'block' }}>
                  错误：{log.error}
                </Text>
              )}
              {log.result && (
                <pre
                  style={{
                    marginTop: 6,
                    background: 'rgba(255,255,255,0.06)',
                    padding: 8,
                    borderRadius: 4,
                    fontSize: 12,
                    maxHeight: 160,
                    overflow: 'auto',
                  }}
                >
                  {log.result}
                </pre>
              )}
            </List.Item>
          )}
        />
      )}
    </Drawer>
  );
}
