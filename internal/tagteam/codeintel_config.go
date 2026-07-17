package tagteam

import (
	"path/filepath"
	"strings"
)

func mergeCodeIntelConfig(dst *CodeIntelConfig, src CodeIntelConfig) {
	if len(src.Providers) > 0 {
		if dst.Providers == nil {
			dst.Providers = map[string]CodeIntelProviderConfig{}
		}
		for name, provider := range src.Providers {
			dst.Providers[name] = provider
		}
	}
	if len(src.AllowedRepos) > 0 {
		dst.AllowedRepos = append([]string(nil), src.AllowedRepos...)
	}
	if len(src.ExcludePaths) > 0 {
		dst.ExcludePaths = append([]string(nil), src.ExcludePaths...)
	}
	if src.Timeout != "" {
		dst.Timeout = src.Timeout
	}
	if src.Dory.Enabled || src.Dory.Path != "" || src.Dory.APIKeyEnv != "" {
		dst.Dory = src.Dory
	}
	if src.Alexandria.Enabled || src.Alexandria.Path != "" || src.Alexandria.APIKeyEnv != "" {
		dst.Alexandria = src.Alexandria
	}
	if src.Muninn.Enabled || src.Muninn.Path != "" || src.Muninn.APIKeyEnv != "" {
		dst.Muninn = src.Muninn
	}
}

func codeIntelRepoAllowed(workdir string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	abs, err := filepath.Abs(workdir)
	if err != nil {
		return false
	}
	for _, candidate := range allowed {
		candidate, err = filepath.Abs(strings.TrimSpace(candidate))
		if err == nil && candidate == abs {
			return true
		}
	}
	return false
}
