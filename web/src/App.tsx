import { useEffect, useState } from 'react';
import { Spin } from 'antd';
import { api } from './api';
import LoginGate from './LoginGate';
import DashboardPage from './pages/dashboard';

type BootState =
  | { phase: 'loading' }
  | { phase: 'login' }
  | { phase: 'app'; authRequired: boolean };

/** 启动流程：查询认证状态 → 登录页 / 看板。 */
export default function App() {
  const [boot, setBoot] = useState<BootState>({ phase: 'loading' });

  useEffect(() => {
    api
      .authStatus()
      .then((s) => {
        if (s.auth_required && !s.logged_in) {
          setBoot({ phase: 'login' });
        } else {
          setBoot({ phase: 'app', authRequired: s.auth_required });
        }
      })
      .catch(() => setBoot({ phase: 'app', authRequired: false }));
  }, []);

  if (boot.phase === 'loading') {
    return (
      <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Spin size="large" />
      </div>
    );
  }
  if (boot.phase === 'login') {
    return <LoginGate onLoggedIn={() => window.location.reload()} />;
  }
  return <DashboardPage authEnabled={boot.authRequired} />;
}
