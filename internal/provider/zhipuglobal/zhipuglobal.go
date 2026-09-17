// Package zhipuglobal is 智谱海外 (Z.ai) — same protocol as zhipu, different
// domains.
package zhipuglobal

import (
	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/zhipu"
)

var endpoints = zhipu.Endpoints{
	CodingBase:    "https://api.z.ai/api/coding/paas/v4",
	AnthropicBase: "https://api.z.ai/api/anthropic",
	MonitorBase:   "https://api.z.ai",
}

// New returns the Z.ai flavored Zhipu client.
func New(cred *provider.Credential, hc provider.HTTPClient) provider.Provider {
	return zhipu.New(endpoints, cred, hc)
}
