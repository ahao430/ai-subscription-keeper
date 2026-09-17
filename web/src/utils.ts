import dayjs from 'dayjs';

export function timeAgo(iso?: string): string {
  if (!iso) return '从未更新';
  const diff = dayjs().diff(dayjs(iso), 'second');
  if (diff < 5) return '刚刚';
  if (diff < 60) return `${diff}秒前`;
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`;
  return `${Math.floor(diff / 86400)}天前`;
}

export function fmtTime(iso?: string): string {
  if (!iso) return '-';
  return dayjs(iso).format('HH:mm:ss');
}

export function fmtDateTime(iso?: string): string {
  if (!iso) return '-';
  return dayjs(iso).format('YYYY-MM-DD HH:mm:ss');
}

export function fmtResetTime(iso?: string): string {
  if (!iso) return '';
  return dayjs(iso).format('MM-DD HH:mm');
}

export function fmtDuration(ms?: number): string {
  if (ms == null) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export function fmtBalance(amount: number, currency: string): string {
  const symbol =
    currency === 'USD' ? '$' : currency === 'CNY' || currency === '¥' ? '¥' : `${currency} `;
  return `${symbol}${amount.toFixed(2)}`;
}

export function statusTagColor(status: string): string {
  switch (status) {
    case 'ok':
    case 'success':
      return 'green';
    case 'error':
    case 'failed':
      return 'red';
    case 'running':
      return 'blue';
    default:
      return 'default';
  }
}

export function statusText(status: string): string {
  switch (status) {
    case 'ok':
      return '正常';
    case 'error':
      return '异常';
    case 'success':
      return '成功';
    case 'failed':
      return '失败';
    case 'running':
      return '执行中';
    default:
      return '未知';
  }
}
