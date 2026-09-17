# 更新日志

本项目的所有显著变更都记录在此文件中。
格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [1.0.0] - 2026-09-17

首个正式版本。

### 新增

- **状态看板**：供应商彩色卡片、多额度维度（剩余百分比 + 重置时间）、按量余额、套餐等级，单卡 / 全部刷新、自动定时刷新（真实查询供应商，30s ~ 1h 可选）、卡片拖拽排序（悬浮跟手效果）、SSE 流式模型测试、原始额度 JSON（自动脱敏）
- **供应商 LOGO**：各供应商官方图标展示于表单与卡片
- **模型服务**：10+ 供应商预设（官方模板 BaseURL 固定不可改）+ 自定义 OpenAI 兼容模板；计费类型（订阅 / 按量计费，按量计费服务不可创建预热任务）；额度展示文案自定义
- **定时任务**：模型预热（流式请求 + 额度验证）与 Webhook（模板变量）两种类型；执行时间三段式选择器（每天 / 每周 / 自定义 Cron + 格式帮助）；时区、失败重试、立即执行
- **通知渠道（10 种）**：钉钉机器人（加签）、飞书机器人、企业微信机器人、Telegram Bot、ntfy 推送（官方服务 + 随机主题）、Bark（iOS 推送）、Gotify（自建）、邮件（QQ / 163 / Gmail / Outlook 快捷模板 + 自定义 SMTP）、Generic Webhook；每个渠道附配置指引（侧边抽屉）
- **多渠道通知**：单个任务可同时绑定多个通知渠道，执行结果并行推送，单渠道失败不影响其他渠道
- **日志系统**：任务执行记录（Attempt 明细）与模型测试日志均持久化 SQLite；支持按任务 / 服务清空与按天数全局清理；删除任务 / 服务自动级联清理
- **系统设置**：全局网络代理（http / https / socks5，作用于全部 AI 请求）+ 连通性测试；日志清理
- **版本接口**：`GET /api/version`、`-version` 参数、设置页版本展示

### 安全

- 凭证（令牌 / System Token / Secret / SMTP 密码）AES-256-GCM 加密存储，密钥派生自 `WARMUP_ENCRYPTION_KEY` 或自动生成的 `.secret.key`
- 原始数据与日志中的 Token、Authorization、api_key 等敏感字段自动脱敏
- API 响应永不回传明文凭证

### 部署

- 单静态二进制（前端 go:embed 内嵌，纯 Go SQLite 无 CGO，零运行时依赖）
- Docker multi-stage 镜像 + docker-compose
- goreleaser 跨平台发布（darwin / linux / windows，amd64 + arm64）
