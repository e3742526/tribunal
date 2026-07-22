// Package adapters invokes bounded read-only model reviewers and evidence workers.
package adapters

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

type Role string

const (
	RoleReviewer Role = "reviewer"
	RoleVoter    Role = "voter"
	RoleEditor   Role = "editor"
	RoleArbiter  Role = "arbiter"
)

type Request struct {
	RunDir          string
	SystemPrompt    string
	Prompt          string
	Schema          string
	SchemaPath      string
	OutputPath      string
	MaxOutputBytes  int64
	MaxOutputTokens int
	TimeoutSeconds  int
	EnvSecrets      map[string]string
}

type Response struct {
	Raw       []byte
	Text      string
	CostUSD   float64
	InputTok  int
	OutputTok int
	Command   []string
}

type VersionInfo struct {
	Adapter  string `json:"adapter"`
	Found    bool   `json:"found"`
	Runnable bool   `json:"runnable"`
	Version  string `json:"version,omitempty"`
	Binary   string `json:"binary,omitempty"`
	Hint     string `json:"hint,omitempty"`
}

type Adapter interface {
	ID() string
	Serialize() bool
	Detect(context.Context) VersionInfo
	Invoke(context.Context, Role, domain.Panelist, Request) (Response, error)
}

// FuncAdapter is an injectable deterministic adapter used by tests, bench, and
// offline demonstrations. Its callback is the complete implementation; it does
// not pretend to represent a vendor result.
type FuncAdapter struct {
	AdapterID string
	Serial    bool
	InvokeFn  func(context.Context, Role, domain.Panelist, Request) (Response, error)
}

func (a *FuncAdapter) ID() string      { return a.AdapterID }
func (a *FuncAdapter) Serialize() bool { return a.Serial }
func (a *FuncAdapter) Detect(context.Context) VersionInfo {
	return VersionInfo{Adapter: a.AdapterID, Found: true, Runnable: true, Version: "in-process"}
}
func (a *FuncAdapter) Invoke(ctx context.Context, role Role, panelist domain.Panelist, req Request) (Response, error) {
	if a.InvokeFn == nil {
		return Response{}, fmt.Errorf("func adapter %q has no callback", a.AdapterID)
	}
	return a.InvokeFn(ctx, role, panelist, req)
}

type Registry struct{ adapters map[string]Adapter }

func NewRegistry(values ...Adapter) *Registry {
	registry := &Registry{adapters: map[string]Adapter{}}
	for _, value := range values {
		registry.adapters[value.ID()] = value
	}
	return registry
}

func (r *Registry) Get(id string) (Adapter, error) {
	adapter, ok := r.adapters[id]
	if !ok {
		return nil, fmt.Errorf("unsupported adapter %q", id)
	}
	return adapter, nil
}

func (r *Registry) Doctor(ctx context.Context) []VersionInfo {
	ids := make([]string, 0, len(r.adapters))
	for id := range r.adapters {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	infos := make([]VersionInfo, 0, len(ids))
	for _, id := range ids {
		infos = append(infos, r.adapters[id].Detect(ctx))
	}
	return infos
}

func detect(ctx context.Context, adapter, binary string) VersionInfo {
	path, err := exec.LookPath(binary)
	if err != nil {
		return VersionInfo{Adapter: adapter, Hint: "install " + binary}
	}
	// A wedged or rogue CLI must not hang doctor or stream unbounded output.
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(callCtx, path, "--version")
	// Without WaitDelay a descendant inheriting the pipe could hold Run open
	// past the kill — the exact doctor hang this timeout exists to prevent.
	cmd.WaitDelay = 5 * time.Second
	output := newBoundedBuffer(64 << 10)
	cmd.Stdout, cmd.Stderr = output, output
	err = cmd.Run()
	return VersionInfo{Adapter: adapter, Found: true, Runnable: err == nil, Version: stringTrim(output.Bytes()), Binary: path, Hint: "authenticate with the provider CLI"}
}
