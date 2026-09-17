// Package bailian implements 阿里云百炼 (DashScope) in OpenAI-compatible mode.
package bailian

import (
	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/openai"
)

func New(cred *provider.Credential, hc provider.HTTPClient) provider.Provider {
	return openai.New(openai.Options{
		DefaultBaseURL:     "https://dashscope.aliyuncs.com/compatible-mode/v1",
		IncludeStreamUsage: true,
		FallbackModels: []string{
			"qwen3-max", "qwen3-coder-plus", "qwen-plus", "qwen-turbo",
		},
	}, cred, hc)
}
