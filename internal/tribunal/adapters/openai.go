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
	Model     string
	APIKeyEnv string
	Headers   map[string]string
	Client    *http.Client
}

func (*OpenAICompatible) ID() string      { return "openai-compatible" }
func (*OpenAICompatible) Serialize() bool { return false }
func (a *OpenAICompatible) Detect(ctx context.Context) VersionInfo {
	info := VersionInfo{Adapter: a.ID(), Found: a.BaseURL != "", Runnable: a.BaseURL != "", Version: "configured"}
	if a.APIKeyEnv != "" && os.Getenv(a.APIKeyEnv) == "" {
		info.Runnable = false
		info.Hint = "set " + a.APIKeyEnv
		return info
	}
	if !info.Runnable {
		return info
	}
	base, err := url.Parse(strings.TrimRight(a.BaseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		info.Runnable = false
		info.Hint = "configure a valid openai-compatible base URL"
		return info
	}
	if a.Model == "" {
		return info
	}
	found, authoritative := a.modelAvailable(ctx, base, a.Model)
	if authoritative && !found {
		info.Runnable = false
		info.Hint = fmt.Sprintf("configured model %q is not advertised by %s/models", a.Model, base.String())
	}
	return info
}

// modelAvailable probes the read-only OpenAI model catalog. Its second return
// value is true only when the endpoint returned a bounded, valid OpenAI model
// list. Some otherwise compatible providers do not implement /models, so an
// unavailable or non-standard catalog must not make doctor reject them.
func (a *OpenAICompatible) modelAvailable(ctx context.Context, base *url.URL, model string) (bool, bool) {
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodGet, base.String()+"/models", nil)
	if err != nil {
		return false, false
	}
	if a.APIKeyEnv != "" {
		httpReq.Header.Set("Authorization", "Bearer "+os.Getenv(a.APIKeyEnv))
	}
	for key, value := range a.Headers {
		httpReq.Header.Set(key, value)
	}
	client := a.httpClient()
	response, err := client.Do(httpReq)
	if err != nil {
		return false, false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, false
	}
	const maxCatalogBytes = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxCatalogBytes+1))
	if err != nil || len(raw) > maxCatalogBytes {
		return false, false
	}
	var catalog struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil || catalog.Data == nil {
		return false, false
	}
	for _, item := range catalog.Data {
		if item.ID == model {
			return true, true
		}
	}
	return false, true
}

func (a *OpenAICompatible) Invoke(ctx context.Context, role Role, panelist domain.Panelist, req Request) (Response, error) {
	base, err := url.Parse(strings.TrimRight(a.BaseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return Response{}, fmt.Errorf("invalid openai-compatible base URL")
	}
	messages := []map[string]string{{"role": "system", "content": req.SystemPrompt}, {"role": "user", "content": req.Prompt}}
	payload := map[string]any{"model": panelist.Model, "messages": messages, "temperature": 0}
	if req.MaxOutputTokens > 0 {
		payload["max_tokens"] = req.MaxOutputTokens
	}
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
	// The redirect guard is overlaid on a shallow copy of any injected
	// client too, so custom transports cannot silently follow a redirect
	// that would repost the document body to another origin.
	client := a.httpClient()
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
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Response{Raw: raw}, fmt.Errorf("decode openai-compatible envelope: %w", err)
	}
	if len(envelope.Choices) == 0 {
		return Response{Raw: raw}, fmt.Errorf("openai-compatible response contained no choices (possibly filtered)")
	}
	content := []byte(envelope.Choices[0].Message.Content)
	text := strings.TrimSpace(string(content))
	if text == "" {
		return Response{Raw: raw, InputTok: envelope.Usage.PromptTokens, OutputTok: envelope.Usage.CompletionTokens, Command: []string{"POST", base.String() + "/chat/completions"}}, fmt.Errorf("openai-compatible response contained empty message content (possibly filtered)")
	}
	return Response{Raw: content, Text: text, InputTok: envelope.Usage.PromptTokens, OutputTok: envelope.Usage.CompletionTokens, Command: []string{"POST", base.String() + "/chat/completions"}}, nil
}

func (a *OpenAICompatible) httpClient() *http.Client {
	client := &http.Client{CheckRedirect: sameOriginRedirect}
	if a.Client != nil {
		clone := *a.Client
		clone.CheckRedirect = sameOriginRedirect
		client = &clone
	}
	return client
}

func sameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 0 && (!strings.EqualFold(req.URL.Host, via[0].URL.Host) || !strings.EqualFold(req.URL.Scheme, via[0].URL.Scheme)) {
		return fmt.Errorf("redirect changed origin or scheme")
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
