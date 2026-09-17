# AI 订阅管家 (AI Subscription Keeper)

🌐 官网：<https://ahao430.github.io/ai-subscription-keeper/>

统一管理你的多个 AI 订阅账号：状态看板、额度/余额监控、模型测试、定时预热、定时 Webhook、结果多渠道通知、全局网络代理。**单二进制部署，数据全部本地存储。**

```
配置一次 AI 账号 → 自动获取模型 → 看板实时查看额度 → 设置定时预热
→ 自动验证额度 → 异常自动重试 → 结果推送到手机
```

## 功能

### 状态看板
- 供应商彩色卡片（官方 LOGO，点击直达官网）、剩余百分比 + 重置时间、按量余额、套餐等级
- 智谱 Coding Plan 卡片带「用量统计」直达链接
- 单卡 / 全部刷新；自动定时刷新（真实查询供应商，30s ~ 1h 可选）
- SSE 流式模型测试，原始额度 JSON 查看（敏感字段自动脱敏）
- 卡片拖拽排序（悬浮跟手动画）

### 模型服务
- 官方预设模板：BaseURL 固定不可改，只填令牌即可
- 自定义 OpenAI 兼容模板：BaseURL 可配置
- 计费类型（订阅 / 按量计费）：按量计费服务不可创建预热任务
- 额度展示文案自定义（如 5h → 5小时限额）

### 定时任务
- **模型预热**：流式请求 + 额度验证，失败自动重试
- **定时提醒**：到点把固定文案推送到选中渠道（"该续费了"这类提醒）
- **Webhook**：模板变量（`{{date}}`、`{{task.name}}`、`{{status}}` 等）
- 执行时间三段式选择：每天几点 / 每周星期几几点 / 自定义 Cron（附格式帮助）
- 时区、重试次数与间隔、立即执行、执行记录（Attempt 明细）

### 通知（9 种渠道，可多选并行推送）
钉钉机器人（加签）· 飞书机器人 · 企业微信机器人 · Telegram Bot · ntfy 推送（官方服务，随机主题）· Bark（iOS）· Gotify（自建）· 邮件（QQ/163/Gmail/Outlook 模板 + 自定义 SMTP）· Generic Webhook

每个渠道内置配置指引（侧边抽屉），单渠道发送失败不影响其他渠道。

### 日志
- 任务执行记录与模型测试日志持久化 SQLite
- 按任务 / 服务清空，或按保留天数全局清理
- 删除任务 / 服务自动级联清理日志

### 系统设置
- 全局网络代理（http/https/socks5，作用于全部 AI 请求）+ 连通性测试
- 配置导入 / 导出（JSON 全量备份，按 ID 合并导入）
- 检查版本更新 + 一键升级（自动下载 Release、替换二进制并重启）
- WebDAV 同步（坚果云模板 + 自建服务器，云端备份 / 恢复）
- 日志清理

## 支持的供应商

| 供应商 | 协议 | 额度查询 |
| --- | --- | --- |
| 智谱（Coding Plan） | OpenAI 兼容 | ✅ 5小时/周限额、MCP月限额、套餐等级 |
| 智谱海外 (Z.ai) | OpenAI 兼容 | ✅ 同上 |
| Kimi For Coding | Anthropic Messages | ✅ 5小时/周限额 |
| Claude | Anthropic Messages | ❌ |
| GPT / Codex | OpenAI 兼容 | ❌ |
| MiniMax | Anthropic Messages | ✅ 5小时/周限额 |
| DeepSeek | OpenAI 兼容 | ✅ 账户余额 |
| 百炼 | OpenAI 兼容 | ❌ |
| NewAPI | OpenAI 兼容 | ✅ 余额（System Token + 用户 ID） |
| Sub2API | OpenAI 兼容 | ❌ |
| 自定义 OpenAI | OpenAI 兼容 | ❌ |

## 安装

### 方式一：下载预编译二进制（推荐）

从 [Releases](https://github.com/ahao430/ai-subscription-keeper/releases/latest) 下载对应平台的压缩包（darwin / linux / windows，amd64 / arm64），解压后直接运行：

```bash
./keeper
# 打开 http://localhost:8080
```

单静态二进制，前端已内嵌，**零运行时依赖**。

### 方式二：Docker

```bash
docker compose up -d
# 打开 http://localhost:8080
```

数据持久化在 volume `keeper-data`（`/data`）。

### 方式三：从源码构建

需要 Go 1.25+ 与 Node.js 20+：

```bash
make build   # 前端编译 → 嵌入 → go build（版本号自动注入）
./bin/keeper
```

本地开发：

```bash
make dev       # 终端 1：后端 :8080
make dev-web   # 终端 2：前端 dev server :5173（/api 代理到 8080）
```

## 环境变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `PORT` | `8080` | HTTP 端口 |
| `DATA_DIR` | `./data` | SQLite 与密钥文件目录 |
| `WARMUP_ENCRYPTION_KEY` | 自动生成 | 凭证加密主密钥（AES-256-GCM） |
| `AUTH_USERNAME` | `admin` | 内置登录用户名 |
| `AUTH_PASSWORD` | 空（免认证） | 设置后启用内置登录（bcrypt 校验 + 会话 + 失败锁定） |

> 未设置 `WARMUP_ENCRYPTION_KEY` 时自动生成并持久化到 `DATA_DIR/.secret.key`；生产环境建议显式设置。

## 安全说明

- 令牌 / System Token / Secret / SMTP 密码等凭证 **AES-256-GCM 加密存储**，API 永不回传明文
- 原始数据与日志中的 Token、Authorization 等敏感字段自动脱敏
- **内置登录（可选）**：设置环境变量 `AUTH_PASSWORD` 后启用——bcrypt 密码校验、HttpOnly 会话 Cookie（7 天滑动有效、HTTPS 自动 Secure）、连续失败 5 次锁定 15 分钟；留空则为免认证模式（本机/内网）
- ⚠️ 公网部署建议：开启内置登录（或反代层 Basic Auth / Cloudflare Access），Docker 端口绑定 `127.0.0.1:8080:8080` 只经反代暴露，并全程 HTTPS

## 技术栈

- 后端：Go 1.25 + SQLite（modernc，纯 Go 无 CGO）+ robfig/cron
- 前端：React 18 + Vite 5 + Ant Design 5（暗色主题）+ @dnd-kit
- 部署：单二进制（go:embed 内嵌前端）/ Docker multi-stage / goreleaser

## 目录结构

```
cmd/server/            入口
internal/
  api/                 REST API + SSE + SPA 静态服务
  app/                 装配层
  config/ crypto/      配置 / AES-256-GCM
  httpclient/          全局代理 + 流式空闲超时
  provider/            统一 Provider 抽象（openai/claude/各厂商实现 + registry）
  scheduler/           Cron 调度（CRON_TZ 时区）
  task/                预热/Webhook 执行器（重试 + 记录 + 多渠道通知）
  notify/              9 种通知渠道发送
  store/               SQLite 存储（含测试日志与级联清理）
  version/ webui/      版本号 / 前端嵌入
web/                   React 前端源码
```

## 开发

```bash
make test     # Go 单元测试
make build    # 构建发布二进制
make docker   # 构建 Docker 镜像
```

发布流程：打 tag（如 `v1.0.1`）推送，GitHub Actions 自动构建多平台产物并发布 Release（CHANGELOG 记得同步更新）。

## 致谢

- [linux.do](https://linux.do/) —— 学AI，上L站

## License

[MIT](./LICENSE)
