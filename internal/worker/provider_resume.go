package worker

import (
	"strings"

	"github.com/gastownhall/gascity/internal/sessionlog"
)

func derivedResumeSessionKey(provider, providerSessionID string) string {
	providerSessionID = strings.TrimSpace(providerSessionID)
	if providerSessionID == "" {
		return ""
	}
	providerFamily := sessionlog.ProviderFamily(provider)
	if providerFamily == "kimi" || providerFamily == "opencode" || providerFamily == "pi" || providerFamily == "antigravity" {
		return providerSessionID
	}
	return ""
}
