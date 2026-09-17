import { useState } from 'react';
import { Button, Card, Form, Input, message } from 'antd';
import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { api } from './api';

/** 登录页：启用内置登录且未认证时全屏展示。 */
export default function LoginGate({ onLoggedIn }: { onLoggedIn: () => void }) {
  const [loading, setLoading] = useState(false);

  async function submit(values: { username: string; password: string }) {
    setLoading(true);
    try {
      await api.login(values.username, values.password);
      message.success('登录成功');
      onLoggedIn();
    } catch (e) {
      message.error(`登录失败：${(e as Error).message}`);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 20,
      }}
    >
      <Card style={{ width: 360 }} styles={{ body: { paddingTop: 28 } }}>
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <img src="/logo.png" alt="logo" style={{ width: 52, height: 52 }} />
          <div style={{ fontSize: 19, fontWeight: 700, marginTop: 10 }}>AI 订阅管家</div>
          <div style={{ fontSize: 13, color: 'rgba(255,255,255,0.55)', marginTop: 4 }}>
            请登录以继续
          </div>
        </div>
        <Form onFinish={submit} size="large">
          <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<UserOutlined />} placeholder="用户名" autoComplete="username" />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password
              prefix={<LockOutlined />}
              placeholder="密码"
              autoComplete="current-password"
            />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={loading}>
            登录
          </Button>
        </Form>
      </Card>
    </div>
  );
}
