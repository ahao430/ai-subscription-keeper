// Package openaivendor is the official OpenAI (GPT / Codex) provider.
// OpenAI exposes no public API-key balance endpoint, so quota is unsupported.
package openaivendor

import (
	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/openai"
)

func New(cred *provider.Credential, hc provider.HTTPClient) provider.Provider {
	return openai.New(openai.Options{
		DefaultBaseURL:     "https://api.openai.com/v1",
		IncludeStreamUsage: true,
	}, cred, hc)
}
