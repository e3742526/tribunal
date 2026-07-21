package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

type Subprocess struct {
	AdapterID string
	Binary    string
	Serial    bool
	ExtraArgs []string
}

func (a *Subprocess) ID() string      { return a.AdapterID }
func (a *Subprocess) Serialize() bool { return a.Serial }
func (a *Subprocess) Detect(ctx context.Context) VersionInfo {
	return detect(ctx, a.AdapterID, a.Binary)
}

func (a *Subprocess) Invoke(ctx context.Context, role Role, panelist domain.Panelist, req Request) (Response, error) {
	if role != RoleReviewer && role != RoleVoter && role != RoleEditor && role != RoleArbiter {
		return Response{}, fmt.Errorf("unsupported Tribunal role %q", role)
	}
	if role == RoleEditor && a.AdapterID == "claude" {
		return Response{}, fmt.Errorf("claude editor is disabled; select codex or agy explicitly")
	}
	binary, err := exec.LookPath(a.Binary)
	if err != nil {
		return Response{}, fmt.Errorf("adapter %s is unavailable: %w", a.AdapterID, err)
	}
	prompt := req.SystemPrompt + "\n\n" + req.Prompt
	argv, stdin, err := a.argv(role, panelist, req, prompt)
	if err != nil {
		return Response{}, err
	}
	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(callCtx, binary, argv...)
	cmd.Dir = req.RunDir
	cmd.Env = restrictedEnv(req.EnvSecrets)
	cmd.Stdin = bytes.NewReader(stdin)
	configureProcess(cmd)
	limit := req.MaxOutputBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	stdout := newBoundedBuffer(limit)
	stderr := newBoundedBuffer(64 << 10)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := runProcess(callCtx, cmd); err != nil {
		return Response{Raw: stdout.Bytes(), Command: append([]string{binary}, argv...)}, fmt.Errorf("%s invocation failed: %w: %s", a.AdapterID, err, redact(string(stderr.Bytes()), req.EnvSecrets))
	}
	raw := stdout.Bytes()
	if req.OutputPath != "" {
		if output, err := os.ReadFile(req.OutputPath); err == nil && len(output) > 0 {
			raw = output
		}
	}
	if a.AdapterID == "claude" {
		raw = unwrapClaude(raw)
	}
	if stdout.Exceeded() || int64(len(raw)) > limit {
		return Response{Raw: raw, Command: append([]string{binary}, argv...)}, fmt.Errorf("%s output exceeded %d bytes", a.AdapterID, limit)
	}
	return Response{Raw: raw, Text: strings.TrimSpace(string(raw)), Command: append([]string{binary}, argv...)}, nil
}

func unwrapClaude(raw []byte) []byte {
	var envelope struct {
		StructuredOutput json.RawMessage `json:"structured_output"`
		Result           json.RawMessage `json:"result"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return raw
	}
	if len(envelope.StructuredOutput) > 0 && string(envelope.StructuredOutput) != "null" {
		return envelope.StructuredOutput
	}
	if len(envelope.Result) > 0 {
		var text string
		if json.Unmarshal(envelope.Result, &text) == nil {
			return []byte(text)
		}
		return envelope.Result
	}
	return raw
}

func (a *Subprocess) argv(role Role, panelist domain.Panelist, req Request, prompt string) ([]string, []byte, error) {
	switch a.AdapterID {
	case "codex":
		args := []string{"exec", "-C", req.RunDir}
		if role == RoleEditor {
			args = append(args, "-s", "read-only") // editor proposes JSON; host alone writes.
		} else {
			args = append(args, "-s", "read-only")
		}
		args = append(args, "-m", panelist.Model)
		if req.SchemaPath != "" {
			args = append(args, "--output-schema", req.SchemaPath)
		}
		if req.OutputPath != "" {
			args = append(args, "-o", req.OutputPath)
		}
		args = append(args, a.ExtraArgs...)
		args = append(args, "-")
		return args, []byte(prompt + "\n"), nil
	case "claude":
		args := []string{"-p", "--model", panelist.Model, "--permission-mode", "dontAsk", "--allowedTools", "", "--output-format", "json"}
		if req.Schema != "" {
			args = append(args, "--json-schema", req.Schema)
		}
		args = append(args, a.ExtraArgs...)
		return args, []byte(prompt + "\n"), nil
	case "agy":
		args := []string{"--print=" + prompt, "--model", panelist.Model, "--print-timeout", strconv.Itoa(req.TimeoutSeconds), "--sandbox", "--mode", "plan"}
		args = append(args, a.ExtraArgs...)
		return args, nil, nil
	default:
		return nil, nil, fmt.Errorf("unsupported subprocess adapter %q", a.AdapterID)
	}
}

func restrictedEnv(secrets map[string]string) []string {
	allowed := map[string]bool{"HOME": true, "PATH": true, "TMPDIR": true, "TMP": true, "TEMP": true, "SHELL": true, "USER": true, "LOGNAME": true, "LANG": true, "LC_ALL": true, "XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true, "XDG_CACHE_HOME": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true}
	var env []string
	for _, pair := range os.Environ() {
		key, _, ok := strings.Cut(pair, "=")
		if ok && allowed[key] {
			env = append(env, pair)
		}
	}
	for key, value := range secrets {
		if key != "" && value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func redact(value string, secrets map[string]string) string {
	for _, secret := range secrets {
		if len(secret) >= 6 {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int64
	exceeded bool
}

func newBoundedBuffer(limit int64) *boundedBuffer { return &boundedBuffer{limit: limit} }
func (b *boundedBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - int64(b.buffer.Len())
	if remaining <= 0 {
		b.exceeded = true
		return len(data), nil
	}
	toWrite := data
	if int64(len(toWrite)) > remaining {
		toWrite = toWrite[:remaining]
		b.exceeded = true
	}
	_, _ = b.buffer.Write(toWrite)
	return len(data), nil
}
func (b *boundedBuffer) Bytes() []byte  { return append([]byte(nil), b.buffer.Bytes()...) }
func (b *boundedBuffer) Exceeded() bool { return b.exceeded }

func stringTrim(value []byte) string { return strings.TrimSpace(string(value)) }

func schemaAndOutputPaths(runDir, invocationID string) (string, string) {
	return filepath.Join(runDir, invocationID+".schema.json"), filepath.Join(runDir, invocationID+".output.json")
}
