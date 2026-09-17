# AGENTS.md — AI 订阅管家开发规范

本文件是给 AI 编码代理（及人类协作者）的项目约定。改动代码前请先读完。

## 项目概述

单二进制的 AI 订阅管理工具：Go 1.25 后端 + React 18 前端（`go:embed` 内嵌）+ SQLite（modernc 纯 Go，无 CGO）。凭证 AES-256-GCM 加密存储。面向本机/内网部署，**无内置登录认证**——不要引入任何假设公网暴露的设计。

## 常用命令

```bash
make dev          # 本地后端 :8080（需前端时另开 make dev-web，Vite 代理 /api）
make dev-web      # 前端 dev server :5173
make build        # 前端编译 → 拷贝进 internal/webui/dist → go build（版本号自动注入 bin/keeper）
make test         # go test ./...（CI 同样执行）
make docker       # 构建镜像
./bin/keeper -version
```

**发布**：更新 CHANGELOG → commit → `git tag vX.Y.Z && git push origin vX.Y.Z`，Actions 自动 goreleaser 出包。小改动升 patch（三位版本号的第三位），功能性改动升 minor。

## 关键约定（容易踩坑）

1. **前端改动必须重新嵌入**：改 `web/` 后要 `cd web && npm run build`，并把 `web/dist/*` 拷到 `internal/webui/dist/` 再 `go build`，否则二进制里还是旧界面。`make build` 已封装。
2. **SQLite 时间列**：modernc 驱动把 TEXT 时间列返回为 string，必须用 `parseTime` / `parseTimePtr`（`internal/store/model_services.go`）手动解析，不要直接 Scan 进 `time.Time`。
3. **Provider 注册表**：新增供应商时改 `internal/provider/registry/registry.go` 的 `types` 切片（表单 schema 的唯一事实源：字段、官网、Logo、额度支持）+ `factories` 映射；实现放 `internal/provider/<name>/`，复用 `openai.Client` 或 `claude.Client`。registry 独立成包是为了避免 import cycle，不要把实现反向 import registry。
4. **凭证安全铁律**：
   - 凭证字段（api_key / system_token / secret / 密码）加密存储，API 响应**永不回传明文**，只回 `*_set: boolean`；
   - 编辑时密钥**留空 = 保持不变**（后端 merge，前端 placeholder 提示）；
   - 原始 HTTP 数据经 `internal/sensitize` 脱敏后才落库/展示；
   - 错误信息里不得包含完整 Token。
5. **Store Create 系列**：`Create*` 在 ID 已预设时保留原 ID（导入/备份恢复依赖此行为，任务按 ID 引用服务与渠道），不要改回无条件覆盖。
6. **任务多渠道通知**：`Task.NotificationChannelIDs []string`，DB 列以逗号分隔存储（历史单值兼容）；逐渠道发送，单渠道失败只打日志不影响其余。
7. **错误与文案**：面向用户的错误信息、注释、UI 文案统一中文；Go 错误用 `fmt.Errorf("动词失败: %w", err)` 风格。

## 前端约定

- Ant Design 5 暗色主题（`main.tsx` ConfigProvider），主色蓝 `#2563eb`，避免紫色系
- 单页看板 + 右上角按钮弹 Modal（`PageModal`），弹窗标题栏加 `paddingRight: 40` 避让关闭按钮
- 全局样式用 `rgba(255,255,255,x)` 系，不要硬编码浅色（暗色下看不清）
- 表单删除类操作：`Popconfirm`；启用状态的条目禁止删除（`disabled={r.enabled}`）
- 供应商 Logo 在 `web/public/logos/*.svg`（白色版，暗色主题用）
- API 调用统一走 `web/src/api.ts` 的 `api` 对象

## 测试与验证

- Provider 额度解析器必须带表驱动单测（参考 `internal/provider/zhipu/zhipu_test.go`）
- 升级/解析等核心纯函数有单测（`internal/api/update_test.go`）
- 提交前 `go vet ./... && go test ./... && cd web && npx tsc --noEmit`

## 提交规范

- Conventional Commits：`feat:` / `fix:` / `chore:` / `docs:`，正文中文说明动机
- 用户可见的改动要在 `CHANGELOG.md` 对应版本小节补条目
- 不要提交：`PLAN.md`、`.zcode/`、`.v2c/`、`data/`、`bin/`、`.env`（.gitignore 已配）
