import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Button,
  Col,
  Modal,
  Row,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import {
  BellOutlined,
  CaretRightOutlined,
  ClockCircleOutlined,
  CloudServerOutlined,
  SettingOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import {
  DndContext,
  DragEndEvent,
  DragOverlay,
  DragStartEvent,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import { restrictToWindowEdges } from '@dnd-kit/modifiers';
import {
  SortableContext,
  arrayMove,
  rectSortingStrategy,
  useSortable,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { api } from '../../api';
import type { DashboardService, ModelService, Task } from '../../types';
import { fmtDateTime, fmtTime, statusTagColor, statusText, timeAgo } from '../../utils';
import ServiceCard from './ServiceCard';
import TestModal from './TestModal';
import RawDrawer, { RawQuotaData } from './RawDrawer';
import ModelServicesPage from '../model-services';
import TasksPage, { cronText } from '../tasks';
import NotificationsPage from '../notifications';
import SettingsPage from '../settings';

const { Text } = Typography;

const REFRESH_OPTIONS = [
  { value: 30, label: '30秒' },
  { value: 60, label: '1分钟' },
  { value: 300, label: '5分钟' },
  { value: 600, label: '10分钟' },
  { value: 1800, label: '30分钟' },
  { value: 3600, label: '1小时' },
  { value: 0, label: '关闭' },
];

function SortableCard(props: {
  service: DashboardService;
  colorIndex: number;
  refreshing: boolean;
  onRefresh: () => void;
  onTest: () => void;
  onRaw: () => void;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: props.service.id,
  });
  return (
    <div
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition: transition ?? 'transform 200ms ease',
        // 拖动中的原位卡片淡化为占位，视觉焦点交给 DragOverlay 悬浮层。
        opacity: isDragging ? 0.35 : 1,
        transitionProperty: 'transform, opacity',
      }}
    >
      <ServiceCard
        {...props}
        dragHandleProps={{ ...attributes, ...listeners }}
      />
    </div>
  );
}

/** 配置弹窗容器：包住原管理页组件，移动端自动收窄。 */
function PageModal({
  open,
  onClose,
  maxWidth = 1180,
  children,
}: {
  open: boolean;
  onClose: () => void;
  maxWidth?: number;
  children: React.ReactNode;
}) {
  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      width="100%"
      style={{ maxWidth, top: 24, paddingBottom: 0 }}
      title={null}
      destroyOnClose
      styles={{ content: { paddingTop: 20 } }}
    >
      {children}
    </Modal>
  );
}

export default function DashboardPage() {
  const [services, setServices] = useState<DashboardService[]>([]);
  const [details, setDetails] = useState<Record<string, ModelService>>({});
  const [tasks, setTasks] = useState<Task[]>([]);
  const [updatedAt, setUpdatedAt] = useState<string>();
  const [loading, setLoading] = useState(false);
  const [refreshingAll, setRefreshingAll] = useState(false);
  const [refreshingId, setRefreshingId] = useState<string | null>(null);
  // 默认开启 30 秒自动刷新。
  const [autoInterval, setAutoInterval] = useState(30);

  const [testSvc, setTestSvc] = useState<DashboardService | null>(null);
  const [rawData, setRawData] = useState<RawQuotaData | null>(null);
  const [rawOpen, setRawOpen] = useState(false);
  const [autoRefreshing, setAutoRefreshing] = useState(false);

  const [svcModalOpen, setSvcModalOpen] = useState(false);
  const [taskModalOpen, setTaskModalOpen] = useState(false);
  const [notifyModalOpen, setNotifyModalOpen] = useState(false);
  const [settingModalOpen, setSettingModalOpen] = useState(false);

  const loadTasks = useCallback(async () => {
    try {
      setTasks(await api.listTasks());
    } catch {
      /* 看板任务区静默失败 */
    }
  }, []);

  const fetchDashboard = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      const resp = await api.dashboard();
      setServices(resp.services);
      setUpdatedAt(resp.updated_at);
      const detailList = await api.listServices();
      const map: Record<string, ModelService> = {};
      for (const d of detailList) map[d.id] = d;
      setDetails(map);
    } catch (e) {
      message.error(`加载看板失败: ${(e as Error).message}`);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchDashboard();
    loadTasks();
  }, [fetchDashboard, loadTasks]);

  // 自动刷新真实查询各供应商额度（POST /api/dashboard/refresh），
  // 而非只读缓存的 GET；静默执行并防止上一轮未完成时叠加请求。
  const autoRefreshingRef = useRef(false);
  const autoRefresh = useCallback(async () => {
    if (autoRefreshingRef.current) return;
    autoRefreshingRef.current = true;
    setAutoRefreshing(true);
    try {
      const resp = await api.refreshDashboard();
      setServices(resp.services);
      setUpdatedAt(resp.updated_at);
    } catch {
      /* 静默失败，下一轮自动重试 */
    } finally {
      autoRefreshingRef.current = false;
      setAutoRefreshing(false);
    }
  }, []);

  const autoRef = useRef<number | null>(null);
  useEffect(() => {
    if (autoRef.current) window.clearInterval(autoRef.current);
    if (autoInterval > 0) {
      autoRef.current = window.setInterval(() => {
        autoRefresh();
        loadTasks();
      }, autoInterval * 1000);
    }
    return () => {
      if (autoRef.current) window.clearInterval(autoRef.current);
    };
  }, [autoInterval, autoRefresh, loadTasks]);

  async function refreshAll() {
    setRefreshingAll(true);
    try {
      const resp = await api.refreshDashboard();
      setServices(resp.services);
      setUpdatedAt(resp.updated_at);
      message.success('全部刷新完成');
    } catch (e) {
      message.error(`刷新失败: ${(e as Error).message}`);
    } finally {
      setRefreshingAll(false);
    }
  }

  async function refreshOne(id: string) {
    setRefreshingId(id);
    try {
      const card = await api.refreshService(id);
      setServices((prev) => prev.map((s) => (s.id === id ? card : s)));
      const detail = await api.getService(id);
      setDetails((prev) => ({ ...prev, [id]: detail }));
    } catch (e) {
      message.error(`刷新失败: ${(e as Error).message}`);
    } finally {
      setRefreshingId(null);
    }
  }

  async function openRaw(svc: DashboardService) {
    try {
      const data = await api.rawQuota(svc.id);
      setRawData(data);
      setRawOpen(true);
    } catch (e) {
      message.error(`获取原始数据失败: ${(e as Error).message}`);
    }
  }

  async function toggleTask(t: Task, enabled: boolean) {
    try {
      await api.updateTask(t.id, { ...t, enabled });
      setTasks((prev) => prev.map((x) => (x.id === t.id ? { ...x, enabled } : x)));
    } catch (e) {
      message.error(`操作失败: ${(e as Error).message}`);
    }
  }

  async function runTaskNow(t: Task) {
    try {
      await api.runTask(t.id);
      message.success(`已触发「${t.name}」执行`);
      setTimeout(loadTasks, 1500);
      setTimeout(loadTasks, 6000);
    } catch (e) {
      message.error(`执行失败: ${(e as Error).message}`);
    }
  }

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }));

  // 拖拽悬浮层：拖住不放时卡片浮起跟随指针。
  const [activeDrag, setActiveDrag] = useState<DashboardService | null>(null);
  const [dragWidth, setDragWidth] = useState(300);

  function onDragStart(event: DragStartEvent) {
    const svc = services.find((s) => s.id === event.active.id) ?? null;
    setActiveDrag(svc);
    const rect = event.active.rect.current.initial ?? event.active.rect.current.translated;
    if (rect?.width) setDragWidth(rect.width);
  }

  function onDragEnd(event: DragEndEvent) {
    setActiveDrag(null);
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = services.findIndex((s) => s.id === active.id);
    const newIndex = services.findIndex((s) => s.id === over.id);
    const next = arrayMove(services, oldIndex, newIndex);
    setServices(next);
    api
      .reorderServices(next.map((s) => s.id))
      .catch((e) => message.error(`保存排序失败: ${(e as Error).message}`));
  }

  const activeDragIndex = activeDrag ? services.findIndex((s) => s.id === activeDrag.id) : -1;

  const enabledTaskCount = tasks.filter((t) => t.enabled).length;

  return (
    <div style={{ minHeight: '100vh' }}>
      {/* 顶栏：logo + 应用名 + 右上角配置入口 */}
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          height: 56,
          padding: '0 16px',
          background: 'rgba(11,16,32,0.72)',
          backdropFilter: 'blur(12px)',
          WebkitBackdropFilter: 'blur(12px)',
          borderBottom: '1px solid rgba(255,255,255,0.09)',
          position: 'sticky',
          top: 0,
          zIndex: 100,
        }}
      >
        <Space size={10}>
          <img src="/logo.png" alt="logo" style={{ width: 30, height: 30, display: 'block' }} />
          <span className="app-title" style={{ fontWeight: 700, fontSize: 16, letterSpacing: 0.3 }}>
            AI 订阅管家
          </span>
        </Space>
        <Space size={2}>
          <Button
            type="text"
            icon={<CloudServerOutlined />}
            onClick={() => setSvcModalOpen(true)}
          >
            <span className="cfg-btn-label">模型服务</span>
          </Button>
          <Button
            type="text"
            icon={<ClockCircleOutlined />}
            onClick={() => setTaskModalOpen(true)}
          >
            <span className="cfg-btn-label">定时任务</span>
          </Button>
          <Button type="text" icon={<BellOutlined />} onClick={() => setNotifyModalOpen(true)}>
            <span className="cfg-btn-label">通知</span>
          </Button>
          <Button type="text" icon={<SettingOutlined />} onClick={() => setSettingModalOpen(true)}>
            <span className="cfg-btn-label">设置</span>
          </Button>
        </Space>
      </div>

      <div style={{ padding: '0 16px 20px' }}>
        {/* 控制条：左侧最后更新；右侧自动刷新 + 全部刷新 */}
        <div
          className="toolbar-row"
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            padding: '12px 0',
            flexWrap: 'wrap',
            gap: 10,
          }}
        >
          <Text type="secondary">
            最后更新：{fmtTime(updatedAt)}
            {autoRefreshing && (
              <SyncOutlined spin style={{ marginLeft: 8, fontSize: 12 }} />
            )}
          </Text>
          <Space wrap>
            <Select
              size="small"
              value={autoInterval}
              onChange={setAutoInterval}
              options={REFRESH_OPTIONS}
              style={{ width: 92 }}
            />
            <Button
              icon={<SyncOutlined spin={refreshingAll || autoRefreshing} />}
              onClick={refreshAll}
              loading={refreshingAll}
            >
              全部刷新
            </Button>
          </Space>
        </div>

        {/* 模型服务卡片 */}
        {loading ? (
          <div style={{ textAlign: 'center', padding: 80 }}>
            <Spin size="large" />
          </div>
        ) : services.length === 0 ? (
          <div
            style={{
              textAlign: 'center',
              padding: 72,
              color: 'rgba(255,255,255,0.55)',
              background: 'rgba(255,255,255,0.05)',
              borderRadius: 12,
              border: '1px dashed rgba(255,255,255,0.18)',
            }}
          >
            还没有模型服务，点击右上角「模型服务」开始配置
          </div>
        ) : (
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            modifiers={[restrictToWindowEdges]}
            onDragStart={onDragStart}
            onDragEnd={onDragEnd}
          >
            <SortableContext items={services.map((s) => s.id)} strategy={rectSortingStrategy}>
              <Row gutter={[12, 12]}>
                {services.map((svc, i) => (
                  <Col key={svc.id} xs={24} sm={12} lg={8} xl={6} xxl={4}>
                    <SortableCard
                      service={svc}
                      colorIndex={i}
                      refreshing={refreshingId === svc.id}
                      onRefresh={() => refreshOne(svc.id)}
                      onTest={() => setTestSvc(svc)}
                      onRaw={() => openRaw(svc)}
                    />
                  </Col>
                ))}
              </Row>
            </SortableContext>
            <DragOverlay
              dropAnimation={{
                duration: 240,
                easing: 'cubic-bezier(0.18, 0.67, 0.6, 1.2)',
              }}
            >
              {activeDrag && activeDragIndex >= 0 && (
                <div
                  style={{
                    width: dragWidth,
                    transform: 'scale(1.04) rotate(1.5deg)',
                    filter: 'drop-shadow(0 18px 36px rgba(2,6,23,0.65))',
                    cursor: 'grabbing',
                  }}
                >
                  <ServiceCard
                    service={activeDrag}
                    colorIndex={activeDragIndex}
                    refreshing={false}
                    onRefresh={() => {}}
                    onTest={() => {}}
                    onRaw={() => {}}
                  />
                </div>
              )}
            </DragOverlay>
          </DndContext>
        )}

        {/* 定时任务区 */}
        <div style={{ marginTop: 20 }}>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              marginBottom: 8,
            }}
          >
            <Space size={10}>
              <span style={{ fontWeight: 600, fontSize: 15 }}>定时任务</span>
              {tasks.length > 0 && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {enabledTaskCount}/{tasks.length} 启用
                </Text>
              )}
            </Space>
            <Button size="small" type="link" onClick={() => setTaskModalOpen(true)}>
              管理任务 →
            </Button>
          </div>
          {tasks.length === 0 ? (
            <div
              style={{
                textAlign: 'center',
                padding: 28,
                color: 'rgba(255,255,255,0.55)',
                background: 'rgba(255,255,255,0.05)',
                borderRadius: 12,
                border: '1px dashed rgba(255,255,255,0.18)',
              }}
            >
              暂无定时任务，点击右上角「定时任务」创建模型预热或 Webhook 任务
            </div>
          ) : (
            <div
              style={{
                background: 'rgba(255,255,255,0.06)',
                border: '1px solid rgba(255,255,255,0.08)',
                borderRadius: 12,
                padding: '4px 12px 12px',
              }}
            >
              <Table
                size="small"
                rowKey="id"
                dataSource={tasks}
                pagination={false}
                scroll={{ x: 860 }}
                columns={[
                  {
                    title: '任务',
                    dataIndex: 'name',
                    ellipsis: true,
                    render: (v: string, r) => (
                      <Space size={8}>
                        {r.type === 'warmup' ? (
                          <Tag color="blue">预热</Tag>
                        ) : r.type === 'reminder' ? (
                          <Tag color="orange">提醒</Tag>
                        ) : (
                          <Tag color="purple">Webhook</Tag>
                        )}
                        <span style={{ fontWeight: 500 }}>{v}</span>
                        {r.type === 'warmup' && (
                          <Text type="secondary" style={{ fontSize: 12 }}>
                            {r.model_service_name} · {r.model}
                          </Text>
                        )}
                        {r.type === 'reminder' && (
                          <Text type="secondary" style={{ fontSize: 12 }} ellipsis>
                            {r.prompt}
                          </Text>
                        )}
                      </Space>
                    ),
                  },
                  {
                    title: '执行计划',
                    dataIndex: 'cron',
                    width: 150,
                    render: (v: string) => (
                      <Tooltip title={v}>
                        <Text code>{cronText(v)}</Text>
                      </Tooltip>
                    ),
                  },
                  {
                    title: '下次执行',
                    dataIndex: 'next_run_at',
                    width: 150,
                    render: (v?: string) =>
                      v ? <Text>{fmtDateTime(v)}</Text> : <Text type="secondary">-</Text>,
                  },
                  {
                    title: '最近结果',
                    width: 170,
                    render: (_, r) => {
                      if (r.running) return <Tag color="processing">执行中</Tag>;
                      if (!r.last_execution) return <Text type="secondary">未执行</Text>;
                      const le = r.last_execution;
                      return (
                        <Tooltip title={le.error || le.finished_at || undefined}>
                          <Space size={6}>
                            <Tag color={statusTagColor(le.status)} style={{ marginRight: 0 }}>
                              {statusText(le.status)}
                            </Tag>
                            {le.finished_at && (
                              <Text type="secondary" style={{ fontSize: 12 }}>
                                {timeAgo(le.finished_at)}
                              </Text>
                            )}
                          </Space>
                        </Tooltip>
                      );
                    },
                  },
                  {
                    title: '启用',
                    dataIndex: 'enabled',
                    width: 70,
                    render: (v: boolean, r) => (
                      <Switch size="small" checked={v} onChange={(c) => toggleTask(r, c)} />
                    ),
                  },
                  {
                    title: '',
                    width: 90,
                    render: (_, r) => (
                      <Button
                        size="small"
                        type="primary"
                        ghost
                        icon={<CaretRightOutlined />}
                        disabled={r.running}
                        onClick={() => runTaskNow(r)}
                      >
                        执行
                      </Button>
                    ),
                  },
                ]}
              />
            </div>
          )}
        </div>
      </div>

      {/* 测试 / 原始数据 */}
      <TestModal
        open={!!testSvc}
        dashboard={testSvc}
        detail={testSvc ? details[testSvc.id] ?? null : null}
        onClose={() => setTestSvc(null)}
      />
      <RawDrawer open={rawOpen} data={rawData} onClose={() => setRawOpen(false)} />

      {/* 配置弹窗 */}
      <PageModal
        open={svcModalOpen}
        onClose={() => {
          setSvcModalOpen(false);
          fetchDashboard(true);
        }}
      >
        <ModelServicesPage />
      </PageModal>
      <PageModal
        open={taskModalOpen}
        onClose={() => {
          setTaskModalOpen(false);
          loadTasks();
        }}
      >
        <TasksPage />
      </PageModal>
      <PageModal open={notifyModalOpen} maxWidth={860} onClose={() => setNotifyModalOpen(false)}>
        <NotificationsPage />
      </PageModal>
      <PageModal open={settingModalOpen} maxWidth={680} onClose={() => setSettingModalOpen(false)}>
        <SettingsPage />
      </PageModal>
    </div>
  );
}
