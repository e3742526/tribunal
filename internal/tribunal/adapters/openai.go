package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

type OpenAICompatible struct {
	BaseURL   string
	APIKeyEnv string
	Headers   map[string]string
	Client    *http.Client
}

func (*OpenAICompatible) ID() string      { return "openai-compatible" }
func (*OpenAICompatible) Serialize() bool { return false }
func (a *OpenAICompatible) Detect(context.Context) VersionInfo {
	info := VersionInfo{Adapter: a.ID(), Found: a.BaseURL != "", Runnable: a.BaseURL != "", Version: "configured"}
	if a.APIKeyEnv != "" && os.Getenv(a.APIKeyEnv) == "" {
		info.Runnable = false
		info.Hint = "set " + a.APIKeyEnv
	}
	return info
}

func (a *OpenAICompatible) Invoke(ctx context.Context, role Role, panelist domain.Panelist, req Request) (Response, error) {
	base, err := url.Parse(strings.TrimRight(a.BaseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return Response{}, fmt.Errorf("invalid openai-compatible base URL")
	}
	messages := []map[string]string{{"role": "system", "content": req.SystemPrompt}, {"role": "user", "content": req.Prompt}}
	payload := map[string]any{"model": panelist.Model, "messages": messages, "temperature": 0}
	if req.Schema != "" {
		var schema any
		if err := json.Unmarshal([]byte(req.Schema), &schema); err != nil {
			return Response{}, fmt.Errorf("invalid request schema: %w", err)
		}
		payload["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": string(role), "strict": true, "schema": schema}}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, durationSeconds(req.TimeoutSeconds))
	defer cancel()
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, base.String()+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if a.APIKeyEnv != "" {
		key := os.Getenv(a.APIKeyEnv)
		if key == "" {
			return Response{}, fmt.Errorf("openai-compatible credential %s is not set", a.APIKeyEnv)
		}
		httpReq.Header.Set("Authorization", "Bearer "+key)
	}
	for key, value := range a.Headers {
		httpReq.Header.Set(key, value)
	}
	client := a.Client
	if client == nil {
		client = &http.Client{CheckRedirect: sameOriginRedirect}
	}
	response, err := client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("openai-compatible request: %w", err)
	}
	defer response.Body.Close()
	limit := req.MaxOutputBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return Response{}, err
	}
	if int64(len(raw)) > limit {
		return Response{}, fmt.Errorf("openai-compatible output exceeded %d bytes", limit)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Response{Raw: raw}, fmt.Errorf("openai-compatible status %d: %s", response.StatusCode, redact(string(raw), req.EnvSecrets))
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Choices) == 0 {
		return Response{Raw: raw}, fmt.Errorf("decode openai-compatible envelope: %w", err)
	}
	content := []byte(envelope.Choices[0].Message.Content)
	return Response{Raw: content, Text: strings.TrimSpace(string(content)), InputTok: envelope.Usage.PromptTokens, OutputTok: envelope.Usage.CompletionTokens, Command: []string{"POST", base.String() + "/chat/completions"}}, nil
}

func sameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 0 && !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		return fmt.Errorf("redirect changed origin")
	}
	if len(via) >= 3 {
		return fmt.Errorf("too many redirects")
	}
	return nil
}

func durationSeconds(value int) time.Duration {
	if value <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(value) * time.Second
}
