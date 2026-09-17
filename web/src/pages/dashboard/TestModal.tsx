import { useEffect, useRef, useState } from 'react';
import { Alert, Descriptions, Input, Modal, Select, Space, Tag, Typography } from 'antd';
import { LoadingOutlined } from '@ant-design/icons';
import { streamModelTest } from '../../api';
import type { DashboardService, ModelService, StreamResult } from '../../types';
import { fmtDuration } from '../../utils';

const { Text, Paragraph } = Typography;

export default function TestModal({
  open,
  dashboard,
  detail,
  onClose,
}: {
  open: boolean;
  dashboard: DashboardService | null;
  detail: ModelService | null;
  onClose: () => void;
}) {
  const [model, setModel] = useState('');
  const [prompt, setPrompt] = useState('hi');
  const [content, setContent] = useState('');
  const [status, setStatus] = useState<'idle' | 'streaming' | 'done' | 'error'>('idle');
  const [result, setResult] = useState<StreamResult | null>(null);
  const [error, setError] = useState('');
  const startedRef = useRef(false);

  useEffect(() => {
    if (open && dashboard && detail) {
      setModel(dashboard.default_test_model || detail.default_test_model || '');
      setPrompt(detail.default_test_prompt || 'hi');
      reset();
      startedRef.current = false;
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, dashboard?.id, detail?.id]);

  function reset() {
    setContent('');
    setStatus('idle');
    setResult(null);
    setError('');
  }

  function run() {
    if (!dashboard || status === 'streaming') return;
    reset();
    setStatus('streaming');
    startedRef.current = true;
    streamModelTest(dashboard.id, { model, prompt }, (ev) => {
      if (ev.type === 'delta' && ev.content) {
        setContent((c) => c + ev.content);
      } else if (ev.type === 'done') {
        setResult(ev.result ?? null);
        if (ev.error) {
          setError(ev.error);
          setStatus('error');
        } else {
          setStatus('done');
        }
      } else if (ev.type === 'error') {
        setError(ev.error ?? '未知错误');
        setStatus('error');
      }
    });
  }

  const models = detail?.models ?? [];

  return (
    <Modal
      title={`模型测试 - ${dashboard?.name ?? ''}`}
      open={open}
      onCancel={() => {
        if (status !== 'streaming') onClose();
      }}
      onOk={run}
      okText="开始测试"
      okButtonProps={{ disabled: !model || status === 'streaming' }}
      cancelButtonProps={{ disabled: status === 'streaming' }}
      width={640}
      destroyOnClose
    >
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        <Space wrap>
          <Select
            style={{ minWidth: 220 }}
            placeholder="测试模型"
            value={model || undefined}
            onChange={setModel}
            showSearch
            options={models.map((m) => ({ value: m, label: m }))}
          />
          <Input
            style={{ width: 240 }}
            placeholder="提示词"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
          />
        </Space>

        {status === 'streaming' && (
          <div>
            <Text type="secondary">
              <LoadingOutlined /> 正在响应...
            </Text>
          </div>
        )}

        {(content || status === 'streaming') && (
          <div
            style={{
              background: 'rgba(255,255,255,0.06)',
              border: '1px solid rgba(255,255,255,0.12)',
              borderRadius: 6,
              padding: 12,
              maxHeight: 260,
              overflow: 'auto',
              whiteSpace: 'pre-wrap',
              fontSize: 13,
            }}
          >
            {content || <Text type="secondary">等待响应…</Text>}
          </div>
        )}

        {error && <Alert type="error" message={error} showIcon />}

        {status === 'done' && result && (
          <Descriptions size="small" column={2} bordered>
            <Descriptions.Item label="请求状态">
              <Tag color="green">成功</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="模型">{result.model}</Descriptions.Item>
            <Descriptions.Item label="请求耗时">{fmtDuration(result.duration_ms)}</Descriptions.Item>
            <Descriptions.Item label="Token 用量">
              {result.usage
                ? `${result.usage.total_tokens}（输入 ${result.usage.prompt_tokens} / 输出 ${result.usage.completion_tokens}）`
                : '未返回'}
            </Descriptions.Item>
          </Descriptions>
        )}
      </Space>
    </Modal>
  );
}
