package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func TestMistralAcpDefaultsBinaryAndSessionMode(t *testing.T) {
	a := &MistralAcp{}
	if got := a.binary(); got != "vibe-acp" {
		t.Fatalf("binary() = %q", got)
	}
	if got := a.sessionMode(); got != "plan" {
		t.Fatalf("sessionMode() = %q", got)
	}
	if a.ID() != "mistral-acp" {
		t.Fatalf("ID() = %q", a.ID())
	}
}

func TestMistralAcpDetectMissingBinary(t *testing.T) {
	a := &MistralAcp{Binary: "tribunal-test-nonexistent-vibe-acp"}
	info := a.Detect(context.Background())
	if info.Found || info.Runnable {
		t.Fatalf("missing binary should not be found/runnable: %#v", info)
	}
}

func TestAcpMessageChunkText(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"bare string content", `{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","content":"hello"}}`, "hello"},
		{"object content", `{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"hi there"}}}`, "hi there"},
		{"non message update ignored", `{"sessionId":"s","update":{"sessionUpdate":"plan"}}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var update acpSessionUpdate
			if err := json.Unmarshal([]byte(tc.raw), &update); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := acpMessageChunkText(update); got != tc.want {
				t.Fatalf("acpMessageChunkText() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSelectACPPermissionOutcomePrefersAllowOnce(t *testing.T) {
	params := json.RawMessage(`{"options":[{"optionId":"reject-1","kind":"reject_once"},{"optionId":"allow-always-1","kind":"allow_always"},{"optionId":"allow-once-1","kind":"allow_once"}]}`)
	outcome := selectACPPermissionOutcome(params)
	inner, ok := outcome["outcome"].(map[string]any)
	if !ok || inner["outcome"] != "selected" || inner["optionId"] != "allow-once-1" {
		t.Fatalf("outcome = %#v", outcome)
	}
}

// --- acpRPC transport --------------------------------------------------

func TestACPRPCCallMatchesResponseByID(t *testing.T) {
	agentReader, clientWriter := io.Pipe()
	clientReader, agentWriter := io.Pipe()
	rpc := newACPRPC(clientWriter)
	go func() { _ = rpc.serve(clientReader) }()

	go func() {
		reader := bufio.NewReader(agentReader)
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		var req acpEnvelope
		_ = json.Unmarshal([]byte(line), &req)
		resp := acpEnvelope{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"ok":true}`)}
		b, _ := json.Marshal(resp)
		_, _ = agentWriter.Write(append(b, '\n'))
	}()

	result, err := rpc.call(context.Background(), "ping", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("call() error = %v", err)
	}
	if string(result) != `{"ok":true}` {
		t.Fatalf("result = %s", result)
	}
}

func TestACPRPCServeRejectsPendingCallOnEOF(t *testing.T) {
	agentReader, clientWriter := io.Pipe()
	clientReader, agentWriter := io.Pipe()
	rpc := newACPRPC(clientWriter)
	go func() { _ = rpc.serve(clientReader) }()
	go func() { _, _ = io.Copy(io.Discard, agentReader) }()

	errCh := make(chan error, 1)
	go func() {
		_, err := rpc.call(context.Background(), "ping", nil)
		errCh <- err
	}()
	time.Sleep(20 * time.Millisecond)
	_ = agentWriter.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected an error when the peer's stream ends")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call() did not return after peer EOF")
	}
}

// --- Invoke() against a fake vibe-acp subprocess ------------------------
//
// TestMistralAcpFakeAgentHelperProcess re-executes this test binary as the
// ACP agent subprocess (the os/exec-style TestHelperProcess pattern). It is
// a no-op under a normal `go test` run.
//
// restrictedEnv() only forwards a fixed host-environment allowlist (see
// subprocess.go) — by design, since a real vibe-acp needs no adapter-passed
// secrets or flags — so there is no environment-variable channel available
// to tell a child process "you are the fake agent, behave like X". Instead
// the mode travels through the one channel Invoke does control: cmd.Dir is
// set to req.RunDir, so fakeMistralAcpAdapter writes it to a marker file
// there before invoking, and this helper reads that same file relative to
// its own (inherited) working directory.

const mistralAcpFakeModeMarker = ".tribunal-mistral-acp-fake-mode"

func TestMistralAcpFakeAgentHelperProcess(t *testing.T) {
	data, err := os.ReadFile(mistralAcpFakeModeMarker)
	if err != nil {
		return
	}
	runMistralAcpFakeAgent(string(data))
	os.Exit(0)
}

func runMistralAcpFakeAgent(mode string) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	write := func(v map[string]any) {
		v["jsonrpc"] = "2.0"
		b, err := json.Marshal(v)
		if err != nil {
			return
		}
		_, _ = os.Stdout.Write(append(b, '\n'))
	}
	for scanner.Scan() {
		var req struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		switch req.Method {
		case "initialize":
			write(map[string]any{"id": req.ID, "result": map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "authMethods": []any{}}})
		case "session/new":
			write(map[string]any{"id": req.ID, "result": map[string]any{"sessionId": "fake-session-1"}})
		case "session/set_mode":
			if mode == "set_mode_error" {
				write(map[string]any{"id": req.ID, "error": map[string]any{"code": -32001, "message": "set_mode not supported"}})
				continue
			}
			write(map[string]any{"id": req.ID, "result": map[string]any{}})
		case "session/set_model":
			write(map[string]any{"id": req.ID, "result": map[string]any{}})
		case "session/prompt":
			fakeAgentRespondToPrompt(mode, req.ID, write)
		}
	}
}

func fakeAgentRespondToPrompt(mode string, promptID *int64, write func(map[string]any)) {
	update := func(content any) {
		write(map[string]any{"method": "session/update", "params": map[string]any{
			"sessionId": "fake-session-1",
			"update":    map[string]any{"sessionUpdate": "agent_message_chunk", "content": content},
		}})
	}
	switch mode {
	case "refusal":
		write(map[string]any{"id": promptID, "result": map[string]any{"stopReason": "refusal"}})
	case "flood":
		update(map[string]any{"type": "text", "text": strings.Repeat("x", 64*1024)})
		write(map[string]any{"id": promptID, "result": map[string]any{"stopReason": "end_turn"}})
	default: // "review"
		update(map[string]any{"type": "text", "text": `{"schema_version":1,"reviewer_id":"R-001","summary":"fake review summary","findings":[]}`})
		write(map[string]any{"id": promptID, "result": map[string]any{"stopReason": "end_turn"}})
	}
}

// fakeMistralAcpAdapter returns a MistralAcp adapter whose "binary" is this
// test binary itself, restricted via -test.run to only ever execute
// TestMistralAcpFakeAgentHelperProcess. It does not write the mode marker —
// callers must write mistralAcpFakeModeMarker into the Request.RunDir they
// pass to Invoke before calling it, since that directory becomes the child
// process's cwd.
func fakeMistralAcpAdapter(t *testing.T) *MistralAcp {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable(): %v", err)
	}
	return &MistralAcp{Binary: self, ExtraArgs: []string{"-test.run=^TestMistralAcpFakeAgentHelperProcess$"}}
}

func writeFakeMode(t *testing.T, runDir, mode string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(runDir, mistralAcpFakeModeMarker), []byte(mode), 0o600); err != nil {
		t.Fatalf("write fake mode marker: %v", err)
	}
}

func TestMistralAcpInvokeReviewerRole(t *testing.T) {
	adapter := fakeMistralAcpAdapter(t)
	runDir := t.TempDir()
	writeFakeMode(t, runDir, "review")
	response, err := adapter.Invoke(context.Background(), RoleReviewer, domain.Panelist{Model: "mistral-large-latest"}, Request{
		RunDir:         runDir,
		Prompt:         "review this packet",
		TimeoutSeconds: 10,
		MaxOutputBytes: 1 << 20,
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !strings.Contains(response.Text, "fake review summary") {
		t.Fatalf("response = %#v", response)
	}
}

func TestMistralAcpInvokeVoterAndArbiterRolesSupported(t *testing.T) {
	for _, role := range []Role{RoleVoter, RoleArbiter, RoleEditor} {
		adapter := fakeMistralAcpAdapter(t)
		runDir := t.TempDir()
		writeFakeMode(t, runDir, "review")
		_, err := adapter.Invoke(context.Background(), role, domain.Panelist{Model: "mistral-large-latest"}, Request{
			RunDir:         runDir,
			Prompt:         "packet",
			TimeoutSeconds: 10,
			MaxOutputBytes: 1 << 20,
		})
		if err != nil {
			t.Fatalf("Invoke(%s) error = %v", role, err)
		}
	}
}

func TestMistralAcpInvokeFailsWhenReadOnlyModeCannotBeSelected(t *testing.T) {
	adapter := fakeMistralAcpAdapter(t)
	runDir := t.TempDir()
	writeFakeMode(t, runDir, "set_mode_error")
	_, err := adapter.Invoke(context.Background(), RoleReviewer, domain.Panelist{Model: "mistral-large-latest"}, Request{
		RunDir:         runDir,
		Prompt:         "review this packet",
		TimeoutSeconds: 10,
		MaxOutputBytes: 1 << 20,
	})
	if err == nil {
		t.Fatal("expected an error when session/set_mode fails")
	}
	if !strings.Contains(err.Error(), "read-only session mode") {
		t.Fatalf("error = %v, want a read-only-mode failure, not a silent fallback into an unknown mode", err)
	}
}

func TestMistralAcpInvokeBoundsStreamedTranscript(t *testing.T) {
	adapter := fakeMistralAcpAdapter(t)
	runDir := t.TempDir()
	writeFakeMode(t, runDir, "flood")
	_, err := adapter.Invoke(context.Background(), RoleReviewer, domain.Panelist{Model: "mistral-large-latest"}, Request{
		RunDir:         runDir,
		Prompt:         "review this packet",
		TimeoutSeconds: 10,
		MaxOutputBytes: 256,
	})
	if err == nil {
		t.Fatal("expected an output-limit error for a flooding agent")
	}
	if !strings.Contains(err.Error(), "mistral-acp") || !strings.Contains(err.Error(), "output exceeded") {
		t.Fatalf("error = %v, want an output-limit error naming mistral-acp", err)
	}
}

func TestMistralAcpInvokeSurfacesRefusal(t *testing.T) {
	adapter := fakeMistralAcpAdapter(t)
	runDir := t.TempDir()
	writeFakeMode(t, runDir, "refusal")
	_, err := adapter.Invoke(context.Background(), RoleReviewer, domain.Panelist{Model: "mistral-large-latest"}, Request{
		RunDir:         runDir,
		Prompt:         "review this packet",
		TimeoutSeconds: 10,
		MaxOutputBytes: 1 << 20,
	})
	if err == nil {
		t.Fatal("expected an error for a refused turn with no assistant text")
	}
	if !strings.Contains(err.Error(), "refusal") {
		t.Fatalf("error = %v", err)
	}
}
