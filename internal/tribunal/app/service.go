// Package app implements Tribunal's application use cases.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/adapters"
	"github.com/e3742526/tribunal/internal/tribunal/config"
	"github.com/e3742526/tribunal/internal/tribunal/documents"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

const (
	ExitSuccess          = 0
	ExitBlockingFindings = 1
	ExitArbitration      = 2
	ExitDegraded         = 3
	ExitInvalidArguments = 4
	ExitPreflight        = 5
	ExitAborted          = 6
)

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}
func (e *ExitError) Unwrap() error { return e.Err }

type Service struct {
	Config        config.Config
	Store         *storage.Store
	Registry      *adapters.Registry
	Clock         func() time.Time
	EvidenceFetch func(context.Context, adapters.EvidenceTarget, []string, map[string]string) (domain.EvidenceItem, error)
	EditFault     func(string) error
}

type ReviewOptions struct {
	Input        string
	Kind         string
	Panel        string
	PanelValue   *domain.Panel
	Split        bool
	FailOnSecret bool
	KnownSecrets []string
	RunID        string
	RunDir       string
	Workspace    *storage.Workspace
	Packet       *documents.Packet
	ReplayOf     string
	NoWorkers    bool
}

type Meta struct {
	SchemaVersion  int          `json:"schema_version"`
	RunID          string       `json:"run_id"`
	WorkspaceID    string       `json:"workspace_id"`
	InputRoot      string       `json:"input_root"`
	PacketHash     string       `json:"packet_hash"`
	Panel          domain.Panel `json:"panel"`
	DiversityNote  string       `json:"diversity_note"`
	TrustedSources []string     `json:"trusted_config_sources"`
	IgnoredSources []string     `json:"ignored_config_sources"`
	ReplayOf       string       `json:"replay_of,omitempty"`
	NoWorkers      bool         `json:"no_workers"`
	StartedAt      time.Time    `json:"started_at"`
}

func New(cfg config.Config, store *storage.Store, registry *adapters.Registry) (*Service, error) {
	if store == nil || registry == nil {
		return nil, fmt.Errorf("store and adapter registry are required")
	}
	return &Service{Config: cfg, Store: store, Registry: registry, Clock: time.Now}, nil
}

func DefaultRegistry(cfg config.Config) *adapters.Registry {
	return adapters.NewRegistry(
		&adapters.Subprocess{AdapterID: "codex", Binary: "codex"},
		&adapters.Subprocess{AdapterID: "claude", Binary: "claude", Serial: true},
		&adapters.Subprocess{AdapterID: "agy", Binary: "agy"},
		&adapters.OpenAICompatible{BaseURL: cfg.OpenAICompatible.BaseURL, APIKeyEnv: cfg.OpenAICompatible.APIKeyEnv, Headers: cfg.OpenAICompatible.Headers},
	)
}

func documentRoot(input string) (string, error) {
	info, err := os.Stat(input)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return input, nil
	}
	return filepath.Dir(input), nil
}

func (s *Service) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock().UTC()
}

func exitError(code int, format string, args ...any) error {
	return &ExitError{Code: code, Err: fmt.Errorf(format, args...)}
}

func withRunTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = time.Hour
	}
	return context.WithTimeout(ctx, timeout)
}
