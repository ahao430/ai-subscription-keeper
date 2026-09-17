import { Button, Descriptions, Drawer, Empty, Typography, message } from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import type { RawResponseLite } from '../../api';
import { fmtDateTime } from '../../utils';

const { Text } = Typography;

export interface RawQuotaData {
  service_name: string;
  unsupported: boolean;
  raw?: RawResponseLite;
  last_quota_at?: string;
  last_quota_error?: string;
}

export default function RawDrawer({
  open,
  data,
  onClose,
}: {
  open: boolean;
  data: RawQuotaData | null;
  onClose: () => void;
}) {
  const raw = data?.raw;
  return (
    <Drawer
      title={`原始额度响应 - ${data?.service_name ?? ''}`}
      open={open}
      onClose={onClose}
      width={560}
    >
      {!raw ? (
        <Empty description="暂无查询记录，请先刷新额度" />
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <Descriptions size="small" column={1} bordered>
            <Descriptions.Item label="请求">
              {raw.method} {raw.url}
            </Descriptions.Item>
            <Descriptions.Item label="HTTP Status">{raw.http_status}</Descriptions.Item>
            <Descriptions.Item label="查询时间">{fmtDateTime(raw.queried_at)}</Descriptions.Item>
            <Descriptions.Item label="耗时">{raw.duration_ms} ms</Descriptions.Item>
            {raw.error && (
              <Descriptions.Item label="错误">
                <Text type="danger">{raw.error}</Text>
              </Descriptions.Item>
            )}
          </Descriptions>
          <div>
            <div style={{ marginBottom: 8, display: 'flex', justifyContent: 'space-between' }}>
              <Text strong>响应 JSON（已脱敏）</Text>
              <Button
                size="small"
                icon={<CopyOutlined />}
                onClick={() => {
                  navigator.clipboard.writeText(JSON.stringify(raw.body, null, 2));
                  message.success('已复制');
                }}
              >
                复制 JSON
              </Button>
            </div>
            <pre
              style={{
                background: 'rgba(255,255,255,0.06)',
                border: '1px solid rgba(255,255,255,0.12)',
                borderRadius: 6,
                padding: 12,
                maxHeight: 480,
                overflow: 'auto',
                fontSize: 12,
              }}
            >
              {JSON.stringify(raw.body, null, 2)}
            </pre>
          </div>
        </div>
      )}
    </Drawer>
  );
}
