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
  CaretRightOutlined,
  DeleteOutlined,
  EditOutlined,
  HistoryOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import { api } from '../../api';
import type { ModelService, NotificationChannel, Task } from '../../types';
import ExecutionsDrawer from './ExecutionsDrawer';
import TaskModal from './TaskModal';

export default function TasksPage() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [services, setServices] = useState<ModelService[]>([]);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<Task | null>(null);
  const [execTask, setExecTask] = useState<Task | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [ts, svcs, chs] = await Promise.all([
        api.listTasks(),
        api.listServices(),
        api.listChannels(),
      ]);
      setTasks(ts);
      setServices(svcs);
      setChannels(chs);
    } catch (e) {
      message.error(`加载失败: ${(e as Error).message}`);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function runNow(t: Task) {
    try {
      await api.runTask(t.id);
      message.success('已触发执行，稍后可在执行记录中查看');
      setTimeout(load, 1500);
    } catch (e) {
      message.error(`执行失败: ${(e as Error).message}`);
    }
  }

  async function toggleEnabled(t: Task, enabled: boolean) {
    try {
      await api.updateTask(t.id, { ...t, enabled });
      setTasks((prev) => prev.map((x) => (x.id === t.id ? { ...x, enabled } : x)));
    } catch (e) {
      message.error(`操作失败: ${(e as Error).message}`);
    }
  }

  async function remove(t: Task) {
    try {
      await api.deleteTask(t.id);
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
          定时任务
        </Typography.Title>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => {
            setEditing(null);
            setModalOpen(true);
          }}
        >
          新增任务
        </Button>
      </div>

      <Table
        rowKey="id"
        loading={loading}
        dataSource={tasks}
        pagination={false}
        columns={[
          {
            title: '任务',
            dataIndex: 'name',
            render: (v, r) => (
              <Space direction="vertical" size={0}>
                <Typography.Text strong>{v}</Typography.Text>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {r.type === 'warmup'
                    ? `模型预热 · ${r.model_service_name ?? '-'} · ${r.model}`
                    : `Webhook · ${cronText(r.cron)}`}
                </Typography.Text>
              </Space>
            ),
          },
          {
            title: '类型',
            dataIndex: 'type',
            width: 100,
            render: (v: string) =>
              v === 'warmup' ? <Tag color="blue">模型预热</Tag> : <Tag color="purple">Webhook</Tag>,
          },
          {
            title: '执行时间',
            dataIndex: 'cron',
            width: 130,
            render: (v: string, r) => (
              <Typography.Text code>
                {v}
                <br />
                <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                  {r.timezone}
                </Typography.Text>
              </Typography.Text>
            ),
          },
          {
            title: '重试',
            width: 100,
            render: (_, r) => `${r.retry_count} 次 / ${r.retry_interval_min} 分`,
          },
          {
            title: '通知',
            dataIndex: 'notification_channel_names',
            width: 150,
            render: (v: string[] | undefined, r) =>
              v && v.length > 0 ? (
                <span>
                  {v.map((name, i) => (
                    <Tag key={r.id + name} style={{ marginBottom: 2 }}>
                      {name}
                    </Tag>
                  ))}
                </span>
              ) : (
                <Typography.Text type="secondary">不通知</Typography.Text>
              ),
          },
          {
            title: '状态',
            dataIndex: 'running',
            width: 80,
            render: (running: boolean, r) =>
              running ? (
                <Tag color="processing">执行中</Tag>
              ) : (
                <Switch checked={r.enabled} onChange={(v) => toggleEnabled(r, v)} />
              ),
          },
          {
            title: '操作',
            width: 240,
            render: (_, r) => (
              <Space>
                <Button
                  size="small"
                  type="primary"
                  ghost
                  icon={<CaretRightOutlined />}
                  onClick={() => runNow(r)}
                  disabled={r.running}
                >
                  立即执行
                </Button>
                <Button
                  size="small"
                  icon={<HistoryOutlined />}
                  onClick={() => setExecTask(r)}
                >
                  执行记录
                </Button>
                <Button
                  size="small"
                  icon={<EditOutlined />}
                  onClick={() => {
                    setEditing(r);
                    setModalOpen(true);
                  }}
                />
                <Popconfirm title="确认删除该任务？" onConfirm={() => remove(r)} disabled={r.enabled}>
                  <Button size="small" danger icon={<DeleteOutlined />} title="删除" disabled={r.enabled} />
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />

      <TaskModal
        open={modalOpen}
        editing={editing}
        services={services}
        channels={channels}
        onClose={() => setModalOpen(false)}
        onSaved={load}
      />
      <ExecutionsDrawer
        open={!!execTask}
        task={execTask}
        onClose={() => setExecTask(null)}
      />
    </div>
  );
}

export function cronText(cron: string): string {
  const parts = cron.trim().split(/\s+/);
  if (parts.length !== 5) return cron;
  const [min, hour, dom, mon, dow] = parts;
  if (dom === '*' && mon === '*' && dow === '*') {
    if (min.startsWith('*/')) return `每 ${min.slice(2)} 分钟`;
    if (hour === '*') return `每小时第 ${min} 分`;
    return `每天 ${hour}:${min.padStart(2, '0')}`;
  }
  return cron;
}
