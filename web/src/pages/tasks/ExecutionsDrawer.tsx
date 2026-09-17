import { useCallback, useEffect, useState } from 'react';
import {
  Button,
  Descriptions,
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
import type { Execution, ExecutionAttempt, Task } from '../../types';
import { fmtDateTime, fmtDuration, statusTagColor, statusText } from '../../utils';

const { Text } = Typography;

export default function ExecutionsDrawer({
  open,
  task,
  onClose,
}: {
  open: boolean;
  task: Task | null;
  onClose: () => void;
}) {
  const [executions, setExecutions] = useState<Execution[]>([]);
  const [loading, setLoading] = useState(false);
  const [detailId, setDetailId] = useState<string | null>(null);
  const [attempts, setAttempts] = useState<ExecutionAttempt[]>([]);
  const [detailExec, setDetailExec] = useState<Execution | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const load = useCallback(async () => {
    if (!task) return;
    setLoading(true);
    try {
      setExecutions(await api.taskExecutions(task.id));
    } catch (e) {
      message.error(`加载执行记录失败: ${(e as Error).message}`);
    } finally {
      setLoading(false);
    }
  }, [task]);

  useEffect(() => {
    if (open) {
      setDetailId(null);
      load();
    }
  }, [open, load]);

  async function openDetail(id: string) {
    setDetailId(id);
    setDetailLoading(true);
    try {
      const resp = await api.execution(id);
      setDetailExec(resp.execution);
      setAttempts(resp.attempts);
    } catch (e) {
      message.error(`加载详情失败: ${(e as Error).message}`);
    } finally {
      setDetailLoading(false);
    }
  }

  async function clearLogs() {
    if (!task) return;
    try {
      await api.deleteTaskExecutions(task.id);
      message.success('已清空');
      setExecutions([]);
      setDetailId(null);
    } catch (e) {
      message.error(`清空失败: ${(e as Error).message}`);
    }
  }

  return (
    <Drawer
      title={`执行记录 - ${task?.name ?? ''}`}
      open={open}
      onClose={onClose}
      width={620}
      zIndex={1100}
      extra={
        <Space>
          <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>
            刷新
          </Button>
          {executions.length > 0 && (
            <Popconfirm title="确认清空所有执行记录？" onConfirm={clearLogs}>
              <Button danger icon={<DeleteOutlined />}>
                清空记录
              </Button>
            </Popconfirm>
          )}
        </Space>
      }
    >
      {loading && executions.length === 0 ? (
        <div style={{ textAlign: 'center', padding: 60 }}>
          <Spin />
        </div>
      ) : executions.length === 0 ? (
        <Empty description="暂无执行记录" />
      ) : (
        <>
          <List
            dataSource={executions}
            renderItem={(e) => (
              <List.Item
                style={{ cursor: 'pointer', padding: '10px 4px' }}
                onClick={() => openDetail(e.id)}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', width: '100%' }}>
                  <Space size={12}>
                    <Tag color={statusTagColor(e.status)}>{statusText(e.status)}</Tag>
                    <Text>{fmtDateTime(e.started_at)}</Text>
                    <Text type="secondary">
                      耗时{' '}
                      {e.finished_at
                        ? fmtDuration(
                            new Date(e.finished_at).getTime() - new Date(e.started_at).getTime(),
                          )
                        : '-'}
                    </Text>
                  </Space>
                  <Text type="secondary">重试 {Math.max(0, e.attempt_count - 1)} 次</Text>
                </div>
              </List.Item>
            )}
          />
          {detailId && (
            <div
              style={{
                marginTop: 16,
                borderTop: '1px solid rgba(255,255,255,0.12)',
                paddingTop: 16,
              }}
            >
              {detailLoading ? (
                <Spin />
              ) : (
                <>
                  <Text strong>执行详情</Text>
                  <Descriptions size="small" column={2} bordered style={{ marginTop: 8 }}>
                    <Descriptions.Item label="开始">
                      {fmtDateTime(detailExec?.started_at)}
                    </Descriptions.Item>
                    <Descriptions.Item label="结束">
                      {fmtDateTime(detailExec?.finished_at)}
                    </Descriptions.Item>
                    <Descriptions.Item label="尝试次数">
                      {detailExec?.attempt_count}
                    </Descriptions.Item>
                    <Descriptions.Item label="最终状态">
                      <Tag color={statusTagColor(detailExec?.status ?? '')}>
                        {statusText(detailExec?.status ?? '')}
                      </Tag>
                    </Descriptions.Item>
                  </Descriptions>
                  {detailExec?.error && (
                    <div style={{ marginTop: 8 }}>
                      <Text type="danger">错误：{detailExec.error}</Text>
                    </div>
                  )}
                  {attempts.map((a) => (
                    <div
                      key={a.id}
                      style={{
                        marginTop: 12,
                        border: '1px solid rgba(255,255,255,0.12)',
                        borderRadius: 6,
                        padding: 12,
                      }}
                    >
                      <Space size={12}>
                        <Text strong>Attempt {a.attempt}</Text>
                        {a.model_status && (
                          <span>
                            模型请求{' '}
                            <Tag
                              style={{ marginRight: 0 }}
                              color={statusTagColor(a.model_status)}
                            >
                              {a.model_status === 'success' ? '✓' : '✗'}
                            </Tag>
                          </span>
                        )}
                        {a.quota_status && (
                          <span>
                            额度查询{' '}
                            <Tag
                              style={{ marginRight: 0 }}
                              color={statusTagColor(a.quota_status)}
                            >
                              {a.quota_status === 'success' ? '✓' : '✗'}
                            </Tag>
                          </span>
                        )}
                        {a.http_status > 0 && <Tag>HTTP {a.http_status}</Tag>}
                        {a.finished_at && (
                          <Text type="secondary">
                            耗时{' '}
                            {fmtDuration(
                              new Date(a.finished_at).getTime() -
                                new Date(a.started_at).getTime(),
                            )}
                          </Text>
                        )}
                      </Space>
                      {a.error && (
                        <div style={{ marginTop: 6 }}>
                          <Text type="danger" style={{ fontSize: 12 }}>
                            {a.error}
                          </Text>
                        </div>
                      )}
                      {a.result && (
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
                          {a.result}
                        </pre>
                      )}
                    </div>
                  ))}
                </>
              )}
            </div>
          )}
        </>
      )}
    </Drawer>
  );
}
