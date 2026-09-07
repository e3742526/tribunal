package adapters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/e3742526/tribunal/internal/sharedcatalog"
)

// ModelCatalogEntry describes the models one configured adapter exposes.
// Live provider discovery is preferred; the vendored fleet roster (or the
// operator's own configuration) remains visible with an explicit warning when
// discovery is unavailable, so an unreachable CLI never looks like an adapter
// with no models.
type ModelCatalogEntry struct {
	Adapter string   `json:"adapter"`
	Source  string   `json:"source"`
	Default string   `json:"default,omitempty"`
	Models  []string `json:"models,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// ModelDiscovery is one adapter's live answer: where it came from, which
// model the provider currently defaults to, and what it advertises.
type ModelDiscovery struct {
	Source  string
	Default string
	Models  []string
}

// ModelDiscoverer is implemented by adapters that may be able to enumerate
// their own models.
type ModelDiscoverer interface {
	DiscoverModels(ctx context.Context, workdir string) (ModelDiscovery, error)
}

// ErrNoModelListSurface reports that a provider has no model-list command at
// all, which is a fact about the CLI rather than a failure to reach it. The
// catalog keeps the roster fallback silently in that case; every other error
// stays visible so an unreachable or unauthenticated provider is never
// mistaken for one that simply has no list command.
var ErrNoModelListSurface = errors.New("provider exposes no model-list surface")

// DiscoverModels inspects every registered adapter. Providers with a native
// model-list surface are queried concurrently so one slow or wedged provider
// does not serially delay the whole catalog. Defaults supplies the model an
// adapter is configured to use when the provider does not report one.
func (r *Registry) DiscoverModels(ctx context.Context, workdir string, defaults map[string]string) []ModelCatalogEntry {
	ids := make([]string, 0, len(r.adapters))
	for id := range r.adapters {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	entries := make([]ModelCatalogEntry, len(ids))
	var wg sync.WaitGroup
	for index, id := range ids {
		entries[index] = fallbackModelCatalog(id, defaults[id])
		discoverer, ok := r.adapters[id].(ModelDiscoverer)
		if !ok {
			continue
		}
		wg.Add(1)
		go func(index int, discoverer ModelDiscoverer) {
			defer wg.Done()
			live, err := discoverer.DiscoverModels(ctx, workdir)
			if err != nil {
				if !errors.Is(err, ErrNoModelListSurface) {
					entries[index].Error = err.Error()
				}
				return
			}
			entries[index].Source = live.Source
			if strings.TrimSpace(live.Default) != "" {
				entries[index].Default = strings.TrimSpace(live.Default)
			}
			entries[index].Models = uniqueModels(live.Models, entries[index].Default)
		}(index, discoverer)
	}
	wg.Wait()
	return entries
}

// fallbackModelCatalog reports what is known without contacting a provider:
// the vendored fleet roster where it carries entries for the adapter, and
// otherwise only whatever the operator configured.
func fallbackModelCatalog(adapter, configuredDefault string) ModelCatalogEntry {
	models := sharedcatalog.MaintainedModelsFor(adapter)
	source := "maintained"
	if len(models) == 0 {
		source = "config"
	}
	return ModelCatalogEntry{
		Adapter: adapter,
		Source:  source,
		Default: strings.TrimSpace(configuredDefault),
		Models:  uniqueModels(models, configuredDefault),
	}
}

func uniqueModels(models []string, extras ...string) []string {
	result := make([]string, 0, len(models)+len(extras))
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		result = append(result, value)
	}
	for _, model := range models {
		add(model)
	}
	for _, model := range extras {
		add(model)
	}
	return result
}

// DiscoverModels asks a vendor CLI for its own model list. Only the adapters
// whose CLI exposes one answer; the rest report that plainly so the caller
// keeps the roster fallback instead of showing an empty provider.
func (a *Subprocess) DiscoverModels(ctx context.Context, workdir string) (ModelDiscovery, error) {
	switch a.AdapterID {
	case "agy":
		out, err := a.runModelList(ctx, workdir, "models")
		if err != nil {
			return ModelDiscovery{}, err
		}
		models := parseTabularModelList(out)
		if len(models) == 0 {
			return ModelDiscovery{}, fmt.Errorf("agy models returned no parseable model IDs")
		}
		return ModelDiscovery{Source: "cli", Models: models}, nil
	case "grok":
		out, err := a.runModelList(ctx, workdir, "models")
		if err != nil {
			return ModelDiscovery{}, err
		}
		models, defaultModel := parseGrokModelList(out)
		if len(models) == 0 {
			return ModelDiscovery{}, fmt.Errorf("grok models returned no parseable model IDs")
		}
		return ModelDiscovery{Source: "cli", Default: defaultModel, Models: models}, nil
	default:
		return ModelDiscovery{}, fmt.Errorf("%s: %w", a.AdapterID, ErrNoModelListSurface)
	}
}

func (a *Subprocess) runModelList(ctx context.Context, workdir string, args ...string) ([]byte, error) {
	binary, err := exec.LookPath(a.Binary)
	if err != nil {
		return nil, fmt.Errorf("%s model discovery is unavailable: %w", a.AdapterID, err)
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = workdir
	cmd.Env = restrictedEnv()
	configureProcess(cmd)
	output := newBoundedBuffer(256 << 10)
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Run(); err != nil {
		return nil, modelListError(a.AdapterID, output.Bytes(), err)
	}
	return output.Bytes(), nil
}

// parseTabularModelList reads `<model-id>\t<description>` lines, the shape
// `agy models` prints. Lines without a tab carry no model ID and are skipped.
func parseTabularModelList(raw []byte) []string {
	var models []string
	for _, line := range strings.Split(string(raw), "\n") {
		model, _, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if ok {
			models = append(models, strings.TrimSpace(model))
		}
	}
	return uniqueModels(models)
}

// parseGrokModelList reads `grok models` output: a bulleted list plus an
// optional "Default model:" line.
func parseGrokModelList(raw []byte) ([]string, string) {
	var models []string
	defaultModel := ""
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(line, "Default model:"); ok {
			defaultModel = strings.TrimSpace(value)
			continue
		}
		if !strings.HasPrefix(line, "* ") && !strings.HasPrefix(line, "- ") {
			continue
		}
		model := strings.TrimSpace(line[2:])
		model = strings.TrimSpace(strings.TrimSuffix(model, "(default)"))
		models = append(models, model)
	}
	return uniqueModels(models), defaultModel
}

func modelListError(adapter string, output []byte, err error) error {
	detail := strings.TrimSpace(string(output))
	if len(detail) > 512 {
		detail = detail[:512] + "..."
	}
	if detail == "" {
		return fmt.Errorf("%s model discovery failed: %w", adapter, err)
	}
	return fmt.Errorf("%s model discovery failed: %w: %s", adapter, err, detail)
}

// lockedBuffer is a bytes.Buffer safe for one writer goroutine and a
// concurrent reader. boundedBuffer is not: it is only ever read after the
// process it captured has been waited on.
type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

// DiscoverModels reads the model choices Vibe advertises on a fresh ACP
// session. It reuses the same handshake an invocation performs and prompts
// for nothing, so discovery stays a read-only probe.
func (a *MistralAcp) DiscoverModels(ctx context.Context, workdir string) (ModelDiscovery, error) {
	binary, err := exec.LookPath(a.binary())
	if err != nil {
		return ModelDiscovery{}, fmt.Errorf("%s model discovery is unavailable: %w", a.ID(), err)
	}
	procCtx, stopProc := context.WithCancel(ctx)
	defer stopProc()

	cmd := exec.CommandContext(procCtx, binary, a.ExtraArgs...)
	cmd.Dir = workdir
	cmd.Env = restrictedEnv()
	configureProcess(cmd)
	// The diagnostic is read while the child is still running (the process is
	// only reaped in the deferred cleanup below), so os/exec's copy goroutine
	// and this function touch the buffer concurrently.
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return ModelDiscovery{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ModelDiscovery{}, err
	}
	if err := cmd.Start(); err != nil {
		return ModelDiscovery{}, fmt.Errorf("%s model discovery could not start: %w", a.ID(), err)
	}

	rpc := newACPRPC(stdin)
	serveDone := make(chan error, 1)
	go func() { serveDone <- rpc.serve(stdout) }()
	defer func() {
		stopProc()
		_ = stdin.Close()
		<-serveDone
		_ = cmd.Wait()
	}()

	session, err := a.newACPSession(procCtx, rpc, workdir)
	if err != nil {
		return ModelDiscovery{}, fmt.Errorf("%s model discovery %w", a.ID(), err)
	}
	defaultModel, models := acpModelConfig(session.ConfigOptions)
	if len(models) == 0 {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return ModelDiscovery{}, fmt.Errorf("%s session advertised no model options: %s", a.ID(), detail)
		}
		return ModelDiscovery{}, fmt.Errorf("%s session advertised no model options", a.ID())
	}
	return ModelDiscovery{Source: "acp", Default: defaultModel, Models: models}, nil
}
