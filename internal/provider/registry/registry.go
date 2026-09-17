// Package registry maps provider type codes to implementations and exposes
// the static descriptors the frontend renders credential forms from.
package registry

import (
	"fmt"

	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/bailian"
	"ai-subscription-keeper/internal/provider/claude"
	"ai-subscription-keeper/internal/provider/deepseek"
	"ai-subscription-keeper/internal/provider/kimi"
	"ai-subscription-keeper/internal/provider/minimax"
	"ai-subscription-keeper/internal/provider/newapi"
	"ai-subscription-keeper/internal/provider/openai"
	"ai-subscription-keeper/internal/provider/openaivendor"
	"ai-subscription-keeper/internal/provider/sub2api"
	"ai-subscription-keeper/internal/provider/zhipu"
	"ai-subscription-keeper/internal/provider/zhipuglobal"
)

type factory func(cred *provider.Credential, hc provider.HTTPClient) provider.Provider

var zhipuCN = zhipu.Endpoints{
	CodingBase:    "https://open.bigmodel.cn/api/coding/paas/v4",
	AnthropicBase: "https://open.bigmodel.cn/api/anthropic",
	MonitorBase:   "https://open.bigmodel.cn",
}

var types = []provider.ProviderType{
	{
		Code: "zhipu", Name: "智谱", SupportsQuota: true, Official: true, Logo: "/logos/zhipu.svg",
		Website: "https://bigmodel.cn", UsageURL: "https://bigmodel.cn/coding-plan/personal/usage",
		AuthNote: "Coding Plan 令牌（cy- 开头）或 API Key",
		Fields: []provider.Field{
			{Name: "api_key", Label: "令牌", Type: "password", Required: true, Placeholder: "cy-xxxxxxxx 或 API Key"},
		},
	},
	{
		Code: "zhipu-global", Name: "智谱海外", SupportsQuota: true, Official: true, Logo: "/logos/zhipu.svg",
		Website: "https://z.ai", UsageURL: "https://bigmodel.cn/coding-plan/personal/usage",
		AuthNote: "Z.ai Coding Plan 令牌",
		Fields: []provider.Field{
			{Name: "api_key", Label: "令牌", Type: "password", Required: true},
		},
	},
	{
		Code: "claude", Name: "Claude", SupportsQuota: false, Official: true, Logo: "/logos/claude.svg",
		Website: "https://claude.com",
		AuthNote: "API Key 或 Claude Max OAuth 令牌",
		Fields: []provider.Field{
			{Name: "api_key", Label: "令牌", Type: "password", Required: true},
			{Name: "auth_mode", Label: "认证方式", Type: "text", Placeholder: "api_key（默认）或 bearer（OAuth/Max）"},
		},
	},
	{
		Code: "openai", Name: "GPT / Codex", SupportsQuota: false, Official: true, Logo: "/logos/openai.svg",
		Website: "https://platform.openai.com",
		Fields: []provider.Field{
			{Name: "api_key", Label: "令牌", Type: "password", Required: true},
		},
	},
	{
		Code: "minimax", Name: "MiniMax", SupportsQuota: true, Official: true, Logo: "/logos/minimax.svg",
		Website: "https://platform.minimaxi.com",
		AuthNote: "Coding Plan 订阅令牌，额度查询使用同一令牌",
		Fields: []provider.Field{
			{Name: "api_key", Label: "令牌", Type: "password", Required: true},
		},
	},
	{
		Code: "kimi", Name: "Kimi", SupportsQuota: true, Official: true, Logo: "/logos/kimi.svg",
		Website: "https://www.kimi.com",
		AuthNote: "Kimi For Coding 订阅令牌",
		Fields: []provider.Field{
			{Name: "api_key", Label: "令牌", Type: "password", Required: true},
		},
	},
	{
		Code: "newapi", Name: "NewAPI", SupportsQuota: true, Official: false, Logo: "/logos/newapi.svg",
		Website: "https://github.com/Calcium-Ion/new-api",
		AuthNote: "额度查询需要控制台 System Token + 用户 ID",
		Fields: []provider.Field{
			{Name: "api_key", Label: "令牌（sk-）", Type: "password", Required: true},
			{Name: "base_url", Label: "服务地址", Type: "text", Required: true, Placeholder: "https://newapi.example.com/v1"},
			{Name: "system_token", Label: "System Token", Type: "password", Help: "NewAPI 个人中心生成的系统访问令牌，用于查询额度"},
			{Name: "user_id", Label: "用户 ID", Type: "text", Help: "NewAPI 控制台用户 ID"},
		},
	},
	{
		Code: "sub2api", Name: "Sub2API", SupportsQuota: false, Official: false, Logo: "/logos/sub2api.svg",
		Fields: []provider.Field{
			{Name: "api_key", Label: "令牌", Type: "password", Required: true},
			{Name: "base_url", Label: "服务地址", Type: "text", Required: true, Placeholder: "https://your-sub2api/v1"},
		},
	},
	{
		Code: "deepseek", Name: "DeepSeek", SupportsQuota: true, Official: true, Logo: "/logos/deepseek.svg",
		Website: "https://platform.deepseek.com",
		Fields: []provider.Field{
			{Name: "api_key", Label: "令牌", Type: "password", Required: true},
		},
	},
	{
		Code: "bailian", Name: "百炼", SupportsQuota: false, Official: true, Logo: "/logos/bailian.svg",
		Website: "https://bailian.aliyun.com",
		Fields: []provider.Field{
			{Name: "api_key", Label: "令牌", Type: "password", Required: true},
		},
	},
	{
		Code: "custom-openai", Name: "自定义 OpenAI", SupportsQuota: false, Official: false, Logo: "/logos/custom-openai.svg",
		AuthNote: "OpenAI 兼容接口，需填写服务地址和令牌",
		Fields: []provider.Field{
			{Name: "api_key", Label: "令牌", Type: "password", Required: true},
			{Name: "base_url", Label: "服务地址", Type: "text", Required: true, Placeholder: "https://your-api-endpoint/v1"},
		},
	},
}

var factories = map[string]factory{
	"zhipu":        func(c *provider.Credential, h provider.HTTPClient) provider.Provider { return zhipu.New(zhipuCN, c, h) },
	"zhipu-global": zhipuglobal.New,
	"claude":       func(c *provider.Credential, h provider.HTTPClient) provider.Provider { return claude.New(c, h) },
	"openai":       openaivendor.New,
	"minimax":      minimax.New,
	"kimi":         kimi.New,
	"newapi":       func(c *provider.Credential, h provider.HTTPClient) provider.Provider { return newapi.New(c, h) },
	"sub2api":      sub2api.New,
	"deepseek":     func(c *provider.Credential, h provider.HTTPClient) provider.Provider { return deepseek.New(c, h) },
	"bailian":      bailian.New,
	"custom-openai": func(c *provider.Credential, h provider.HTTPClient) provider.Provider {
		return openai.New(openai.Options{}, c, h)
	},
}

// Types returns the static provider type descriptors.
func Types() []provider.ProviderType { return types }

func TypeByCode(code string) (provider.ProviderType, bool) {
	for _, t := range types {
		if t.Code == code {
			return t, true
		}
	}
	return provider.ProviderType{}, false
}

// Get instantiates a provider for the given type code.
func Get(code string, cred *provider.Credential, hc provider.HTTPClient) (provider.Provider, error) {
	f, ok := factories[code]
	if !ok {
		return nil, fmt.Errorf("未知供应商类型: %s", code)
	}
	if cred == nil {
		cred = &provider.Credential{}
	}
	return f(cred, hc), nil
}
