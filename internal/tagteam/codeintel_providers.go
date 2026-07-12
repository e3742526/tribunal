package tagteam

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CodeIntelProviderRegistry intentionally contains only built-in adapter
// names. Commands come from the resolved configuration; no provider is
// downloaded, installed, or configured by Tagteam.
type CodeIntelProviderRegistry struct{}

func NewCodeIntelProviderRegistry() CodeIntelProviderRegistry { return CodeIntelProviderRegistry{} }

func (CodeIntelProviderRegistry) Names() []string {
	return []string{"command", "codebase-memory", "gitnexus"}
}

func (CodeIntelProviderRegistry) New(name, command string, timeout time.Duration) (CodeIntelProvider, error) {
	name = strings.TrimSpace(name)
	switch name {
	case "command", "codebase-memory", "gitnexus":
		return newNamedCommandCodeIntelProvider(name, command, timeout)
	default:
		return nil, fmt.Errorf("unknown code-intel provider %q", name)
	}
}

// ProbeCodeIntelProvider is the externally useful capability probe. It never
// invokes an observation and has the same bounded subprocess behavior as the
// sensor. Named adapters use their ordinary executable availability check;
// providers may expose richer version commands themselves through their CLI.
func ProbeCodeIntelProvider(ctx context.Context, workdir, name, command string, timeout time.Duration) (CodeIntelCapabilities, error) {
	p, err := NewCodeIntelProviderRegistry().New(name, command, timeout)
	if err != nil {
		return CodeIntelCapabilities{}, err
	}
	if err := p.Probe(ctx, workdir); err != nil {
		return CodeIntelCapabilities{}, err
	}
	return p.Capabilities(), nil
}
