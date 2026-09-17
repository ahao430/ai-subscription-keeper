import { Badge, Button, Card, Popover, Progress, Space, Tag, Typography } from 'antd';
import {
  ApiOutlined,
  DatabaseOutlined,
  ReloadOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import type { DashboardService } from '../../types';
import { fmtBalance, fmtResetTime, timeAgo } from '../../utils';

const { Text, Title } = Typography;

/** 彩色渐变卡片调色板，按顺序轮换（无紫色系）。 */
export const CARD_PALETTES = [
  'linear-gradient(135deg, #1d4ed8 0%, #2563eb 100%)', // blue
  'linear-gradient(135deg, #0e7490 0%, #0891b2 100%)', // cyan
  'linear-gradient(135deg, #047857 0%, #14b8a6 100%)', // emerald → teal
  'linear-gradient(135deg, #b45309 0%, #f59e0b 100%)', // amber
  'linear-gradient(135deg, #be123c 0%, #e11d48 100%)', // rose → red
];

const WHITE = '#ffffff';
const WHITE_80 = 'rgba(255,255,255,0.8)';
const WHITE_65 = 'rgba(255,255,255,0.65)';
const WHITE_25 = 'rgba(255,255,255,0.25)';

export default function ServiceCard({
  service,
  colorIndex = 0,
  refreshing,
  onRefresh,
  onTest,
  onRaw,
  dragHandleProps,
  dragging,
}: {
  service: DashboardService;
  colorIndex?: number;
  refreshing: boolean;
  onRefresh: () => void;
  onTest: () => void;
  onRaw: () => void;
  dragHandleProps?: React.HTMLAttributes<HTMLDivElement>;
  dragging?: boolean;
}) {
  const s = service;
  const bg = CARD_PALETTES[colorIndex % CARD_PALETTES.length];

  const ghostBtn = {
    color: WHITE,
    borderColor: 'rgba(255,255,255,0.4)',
    background: 'rgba(255,255,255,0.12)',
  } as const;

  const statusBadge = (
    <Badge
      status={s.status === 'ok' ? 'success' : s.status === 'error' ? 'error' : 'default'}
      text={
        <span style={{ color: WHITE_80 }}>
          {s.status === 'ok' ? '正常' : s.status === 'error' ? '异常' : '未查询'}
        </span>
      }
    />
  );

  const errorPopover =
    s.status === 'error' && s.last_quota_error ? (
      <Popover
        content={
          <div style={{ maxWidth: 360, wordBreak: 'break-all' }}>{s.last_quota_error}</div>
        }
        title="查询错误"
      >
        <span style={{ cursor: 'pointer', fontSize: 12, color: WHITE_80, textDecoration: 'underline dotted' }}>
          查看错误
        </span>
      </Popover>
    ) : null;

  const dims = s.quota?.dimensions ?? [];
  const balance = s.quota?.balance;
  const usage = s.quota?.usage;
  const extra = s.quota?.extra ?? [];

  return (
    <Card
      size="small"
      style={{
        background: bg,
        borderColor: 'rgba(255,255,255,0.28)',
        boxShadow: '0 8px 24px rgba(2,6,23,0.45)',
        color: WHITE,
        opacity: dragging ? 0.55 : 1,
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
      }}
      styles={{
        header: { color: WHITE, borderColor: WHITE_25 },
        body: { flex: 1, display: 'flex', flexDirection: 'column' },
      }}
      title={
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span
            {...dragHandleProps}
            style={{ cursor: 'grab', color: WHITE_65, fontSize: 14 }}
            title="拖拽排序"
          >
            ⠿
          </span>
          <Text strong ellipsis style={{ maxWidth: 150, color: WHITE }}>
            {s.name}
          </Text>
        </div>
      }
      extra={
        <Space size={4}>
          <Button
            type="text"
            size="small"
            icon={refreshing ? <SyncOutlined spin /> : <ReloadOutlined />}
            onClick={onRefresh}
            style={{ color: WHITE }}
          />
        </Space>
      }
    >
      <Space direction="vertical" size={10} style={{ width: '100%', flex: 1 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          {statusBadge}
          {errorPopover}
          <Tag
            style={{
              marginRight: 0,
              background: 'rgba(255,255,255,0.18)',
              color: WHITE,
              border: '1px solid rgba(255,255,255,0.25)',
              display: 'inline-flex',
              alignItems: 'center',
              gap: 4,
            }}
          >
            {s.provider_logo && (
              <img
                src={s.provider_logo}
                alt=""
                style={{ width: 12, height: 12, display: 'inline-block' }}
              />
            )}
            {s.provider_name}
          </Tag>
        </div>

        {!s.enabled && (
          <Tag color="orange" style={{ alignSelf: 'flex-start' }}>
            已停用
          </Tag>
        )}

        {dims.map((d) => {
          const used = d.used_percentage ?? 0;
          const remaining = Math.max(0, Math.min(100, 100 - used));
          return (
            <div key={d.code}>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 2 }}>
                <span style={{ fontSize: 12, color: WHITE_80 }}>{d.display_name}</span>
                {d.reset_at && (
                  <span style={{ fontSize: 12, color: WHITE_80 }}>重置 {fmtResetTime(d.reset_at)}</span>
                )}
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <Progress
                  percent={remaining}
                  showInfo={false}
                  strokeColor={WHITE}
                  trailColor={WHITE_25}
                  size={{ height: 8 }}
                  style={{ flex: 1, marginBottom: 0 }}
                />
                <span style={{ color: WHITE, fontWeight: 600, fontSize: 13, width: 44, textAlign: 'right' }}>
                  {Math.round(remaining)}%
                </span>
              </div>
              <span style={{ fontSize: 11, color: WHITE_65 }}>剩余</span>
            </div>
          );
        })}

        {balance && (
          <div>
            <span style={{ fontSize: 12, color: WHITE_80 }}>当前余额</span>
            <Title level={4} style={{ margin: 0, color: WHITE }}>
              {fmtBalance(balance.amount, balance.currency)}
            </Title>
          </div>
        )}

        {(usage?.today_spend != null || usage?.month_spend != null) && (
          <div style={{ fontSize: 12, color: WHITE_80 }}>
            {usage?.today_spend != null && <div>今日消费 ¥{usage.today_spend.toFixed(2)}</div>}
            {usage?.month_spend != null && <div>本月消费 ¥{usage.month_spend.toFixed(2)}</div>}
          </div>
        )}

        {extra.length > 0 && (
          <div style={{ fontSize: 12, color: WHITE_80 }}>
            {extra.map((m) => (
              <div key={m.label} style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: WHITE_65 }}>{m.label}</span>
                <span>{m.value}</span>
              </div>
            ))}
          </div>
        )}

        {s.unsupported_quota && dims.length === 0 && !balance && (
          <span style={{ fontSize: 12, color: WHITE_65 }}>该供应商暂不支持额度查询</span>
        )}

        <div style={{ marginTop: 'auto' }}>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              marginBottom: 8,
            }}
          >
            {s.default_test_model ? (
              <Tag
                color="blue"
                style={{
                  marginRight: 0,
                  background: 'rgba(255,255,255,0.22)',
                  color: WHITE,
                  border: '1px solid rgba(255,255,255,0.3)',
                }}
              >
                {s.default_test_model}
              </Tag>
            ) : (
              <span style={{ fontSize: 12, color: WHITE_65 }}>未设置默认测试模型</span>
            )}
            <span style={{ fontSize: 12, color: WHITE_65 }}>{timeAgo(s.last_quota_at)}</span>
          </div>
          <Space style={{ width: '100%', justifyContent: 'space-between' }}>
            <Button size="small" icon={<ApiOutlined />} onClick={onTest} style={ghostBtn}>
              测试模型
            </Button>
            <Button size="small" icon={<DatabaseOutlined />} onClick={onRaw} style={ghostBtn}>
              原始数据
            </Button>
          </Space>
        </div>
      </Space>
    </Card>
  );
}
