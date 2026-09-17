import React from 'react';
import ReactDOM from 'react-dom/client';
import { ConfigProvider, theme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import 'antd/dist/reset.css';
import './global.css';
import App from './App';
import 'dayjs/locale/zh-cn';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider
      locale={zhCN}
      theme={{
        algorithm: theme.darkAlgorithm,
        token: {
          colorPrimary: '#2563eb',
          colorInfo: '#0891b2',
          borderRadius: 10,
        },
        components: {
          Table: {
            colorBgContainer: 'transparent',
            headerBg: 'rgba(255,255,255,0.06)',
          },
          Card: {
            colorBgContainer: 'rgba(255,255,255,0.05)',
          },
        },
      }}
    >
      <App />
    </ConfigProvider>
  </React.StrictMode>,
);
