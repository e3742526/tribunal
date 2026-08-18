package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

// MistralAcp drives Mistral's `vibe-acp` binary — the ACP-over-stdio
// entrypoint shipped by the `mistral-vibe` package (`uv tool install
// mistral-vibe`; `vibe --setup` to authenticate) — as an Agent Client
// Protocol (https://agentclientprotocol.com) client.
//
// Every Tribunal adapter is a bounded, single-turn, read-only invocation:
// one prompt in, one text response out, no persistent session across
// calls (see the package doc: "bounded read-only model reviewers and
// evidence workers"). ACP's session model — initialize once, then
// request/response turns over a still-running process — fits that
// shape directly for a single turn; it does not require the long-lived
// multi-turn chat session an interactive coding agent would use. This
// mirrors the same bounded-turn ACP client cephalopod-ai/tagteam added
// for its own read-only reviewer/scout roles (internal/tagteam/adapters_mistral_acp.go),
// which itself was modeled on gosling's and cuttlefish's verified,
// production `vibe-acp` integrations rather than derived independently.
//
// Every role Tribunal has (reviewer, voter, editor, arbiter) is read-only
// from the model's side — even "editor" only proposes typed replacement
// hunks as JSON text; the host alone validates and writes them (see
// internal/tribunal/app/edit.go). So, like the vendor CLI adapters in
// subprocess.go (which force codex/agy into their own read-only sandbox
// modes regardless of role), this adapter requests Vibe's most
// restrictive "plan" session mode for every role rather than gating by
// role the way tagteam's read-only-only adapter does.
type MistralAcp struct {
	// Binary overrides the resolved executable name; defaults to "vibe-acp".
	Binary string
	// SessionMode overrides Vibe's session/set_mode value; defaults to
	// "plan" (Vibe's most restrictive, read-only-exploration mode).
	SessionMode string
	// ExtraArgs is appended to the invocation argv. Unused by config today
	// (mirrors Subprocess.ExtraArgs, which DefaultRegistry likewise leaves
	// unset); exercised by tests driving a fake agent via -test.run.
	ExtraArgs []string
}

func (a *MistralAcp) ID() string      { return "mistral-acp" }
func (a *MistralAcp) Serialize() bool { return false }

func (a *MistralAcp) binary() string {
	if a.Binary != "" {
		return a.Binary
	}
	return "vibe-acp"
}

func (a *MistralAcp) sessionMode() string {
	if a.SessionMode != "" {
		return a.SessionMode
	}
	return "plan"
}

func (a *MistralAcp) Detect(ctx context.Context) VersionInfo {
	return detect(ctx, a.ID(), a.binary())
}

func (a *MistralAcp) Invoke(ctx context.Context, role Role, panelist domain.Panelist, req Request) (Response, error) {
	if role != RoleReviewer && role != RoleVoter && role != RoleEditor && role != RoleArbiter {
		return Response{}, fmt.Errorf("unsupported Tribunal role %q", role)
	}
	binary, err := exec.LookPath(a.binary())
	if err != nil {
		return Response{}, fmt.Errorf("adapter %s is unavailable: %w", a.ID(), err)
	}

	callCtx, cancel := context.WithTimeout(ctx, durationSeconds(req.TimeoutSeconds))
	defer cancel()
	procCtx, stopProc := context.WithCancel(callCtx)
	defer stopProc()

	cmd := exec.CommandContext(procCtx, binary, a.ExtraArgs...)
	cmd.Dir = req.RunDir
	cmd.Env = restrictedEnv()
	configureProcess(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Response{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Response{}, err
	}
	if err := cmd.Start(); err != nil {
		return Response{}, fmt.Errorf("%s invocation failed: %w", a.ID(), err)
	}

	rpc := newACPRPC(stdin)
	limit := req.MaxOutputBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	transcript := &acpBoundedTranscript{limit: limit}
	rpc.onNotify = func(method string, params json.RawMessage) {
		if method != "session/update" {
			return
		}
		var update acpSessionUpdate
		if err := json.Unmarshal(params, &update); err != nil {
			return
		}
		if text := acpMessageChunkText(update); text != "" && !transcript.append(text) {
			// Output limit exceeded: stop the agent rather than let it keep
			// streaming into memory.
			stopProc()
		}
	}
	rpc.onServerRequest = func(method string, params json.RawMessage) (any, error) {
		if method == "session/request_permission" {
			return selectACPPermissionOutcome(params), nil
		}
		return map[string]any{}, nil
	}

	serveDone := make(chan error, 1)
	go func() { serveDone <- rpc.serve(stdout) }()

	prompt := req.SystemPrompt + "\n\n" + req.Prompt
	runErr := a.runTurn(procCtx, rpc, req.RunDir, panelist.Model, prompt, transcript)

	stopProc()
	_ = stdin.Close()
	<-serveDone
	_ = cmd.Wait()

	command := []string{binary}
	if runErr != nil {
		return Response{Raw: []byte(transcript.String()), Command: command}, runErr
	}
	raw := []byte(transcript.String())
	return Response{Raw: raw, Text: strings.TrimSpace(string(raw)), Command: command}, nil
}

// runTurn drives the initialize -> session/new -> session/set_mode ->
// (best-effort session/set_model) -> session/prompt handshake.
//
// A failure to enter the configured read-only session mode is fatal, not
// a tolerated error: this adapter auto-approves whatever the agent asks
// for in session/request_permission (see selectACPPermissionOutcome), so
// silently continuing in an unknown mode after Vibe rejects "plan" would
// defeat that read-only boundary. session/set_model stays best-effort:
// losing model selection is a quality issue, not a safety one.
func (a *MistralAcp) runTurn(ctx context.Context, rpc *acpRPC, cwd, model, prompt string, transcript *acpBoundedTranscript) error {
	if _, err := rpc.call(ctx, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientCapabilities": map[string]any{},
	}); err != nil {
		return fmt.Errorf("%s initialize failed: %w", a.ID(), err)
	}

	newSessionRaw, err := rpc.call(ctx, "session/new", map[string]any{
		"cwd":        cwd,
		"mcpServers": []any{},
	})
	if err != nil {
		return fmt.Errorf("%s session/new failed: %w", a.ID(), err)
	}
	var newSession struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(newSessionRaw, &newSession); err != nil || strings.TrimSpace(newSession.SessionID) == "" {
		return fmt.Errorf("%s session/new returned no sessionId", a.ID())
	}
	sessionID := newSession.SessionID

	if _, err := rpc.call(ctx, "session/set_mode", map[string]any{"sessionId": sessionID, "modeId": a.sessionMode()}); err != nil {
		return fmt.Errorf("%s could not enter read-only session mode %q, refusing to prompt in an unknown mode: %w", a.ID(), a.sessionMode(), err)
	}

	if model != "" {
		_, _ = rpc.call(ctx, "session/set_model", map[string]any{"sessionId": sessionID, "modelId": model})
	}

	promptRaw, err := rpc.call(ctx, "session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": prompt}},
	})
	if err != nil {
		if transcript.Exceeded() {
			return fmt.Errorf("%s output exceeded %d bytes", a.ID(), transcript.limit)
		}
		return fmt.Errorf("%s session/prompt failed: %w", a.ID(), err)
	}
	if transcript.Exceeded() {
		return fmt.Errorf("%s output exceeded %d bytes", a.ID(), transcript.limit)
	}
	var promptResult struct {
		StopReason string `json:"stopReason"`
	}
	_ = json.Unmarshal(promptRaw, &promptResult)

	if strings.TrimSpace(transcript.String()) == "" {
		switch promptResult.StopReason {
		case "refusal", "cancelled":
			return fmt.Errorf("%s turn ended: %s", a.ID(), promptResult.StopReason)
		}
	}
	return nil
}

// acpSessionUpdate is the subset of ACP's session/update notification this
// adapter reads: the discriminant field ("sessionUpdate") and the assistant
// text carried by agent_message_chunk updates.
type acpSessionUpdate struct {
	SessionID string `json:"sessionId"`
	Update    struct {
		Kind    string          `json:"sessionUpdate"`
		Content json.RawMessage `json:"content"`
		Text    json.RawMessage `json:"text"`
	} `json:"update"`
}

func acpMessageChunkText(u acpSessionUpdate) string {
	if u.Update.Kind != "agent_message_chunk" && u.Update.Kind != "agent_message_text" {
		return ""
	}
	if text := acpContentText(u.Update.Content); text != "" {
		return text
	}
	return acpContentText(u.Update.Text)
}

// acpContentText reads a ContentBlock's text whether it arrives as a bare
// JSON string or as {"type":"text","text":"..."}.
func acpContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var block struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &block); err == nil {
		return block.Text
	}
	return ""
}

// acpPermissionOption mirrors PermissionOption from the ACP schema
// (optionId/name/kind), as sent inside a session/request_permission request.
type acpPermissionOption struct {
	OptionID string `json:"optionId"`
	Kind     string `json:"kind"`
}

// selectACPPermissionOutcome auto-approves a session/request_permission
// call, since this adapter only ever runs bounded, unattended turns with no
// human present to prompt. It prefers the least-privileged option Vibe
// offers (a one-shot allow) over a standing allow_always grant.
func selectACPPermissionOutcome(params json.RawMessage) map[string]any {
	var req struct {
		Options []acpPermissionOption `json:"options"`
	}
	_ = json.Unmarshal(params, &req)
	optionID := ""
	for _, kind := range []string{"allow_once", "allow_always"} {
		for _, opt := range req.Options {
			if opt.Kind == kind {
				optionID = opt.OptionID
				break
			}
		}
		if optionID != "" {
			break
		}
	}
	if optionID == "" && len(req.Options) > 0 {
		optionID = req.Options[0].OptionID
	}
	if optionID == "" {
		return map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}
	}
	return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}}
}

// acpBoundedTranscript accumulates streamed session/update text under the
// request's output-byte limit, the same ceiling the vendor CLI and
// OpenAICompatible adapters enforce (boundedBuffer / io.LimitReader). append
// reports whether the chunk fit; once the limit is exceeded no further text
// is retained and the caller is expected to cancel the subprocess rather
// than let a verbose or malfunctioning agent keep streaming into memory.
type acpBoundedTranscript struct {
	mu       sync.Mutex
	buf      strings.Builder
	limit    int64
	exceeded bool
}

func (t *acpBoundedTranscript) append(s string) bool {
	if s == "" {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.exceeded {
		return false
	}
	if int64(t.buf.Len())+int64(len(s)) > t.limit {
		t.exceeded = true
		return false
	}
	t.buf.WriteString(s)
	return true
}

func (t *acpBoundedTranscript) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buf.String()
}

func (t *acpBoundedTranscript) Exceeded() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exceeded
}
