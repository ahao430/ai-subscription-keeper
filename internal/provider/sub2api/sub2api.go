// Package sub2api implements Sub2API gateways (OpenAI-compatible front for
// subscription accounts). The gateway address must be provided via base_url.
package sub2api

import (
	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/openai"
)

func New(cred *provider.Credential, hc provider.HTTPClient) provider.Provider {
	return openai.New(openai.Options{
		IncludeStreamUsage: true,
	}, cred, hc)
}
