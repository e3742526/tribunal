package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/app"
)

func TestReviewCommandCompletesWithoutGitAndWritesOnlyExternalState(t *testing.T) {
	documentDir := t.TempDir()
	documentPath := filepath.Join(documentDir, "brief.md")
	original := "# Brief\n\nThe launch date is unsupported.\n"
	if err := os.WriteFile(documentPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				JSONSchema struct {
					Name string `json:"name"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		reviewer := map[string]string{"one": "R-001", "two": "R-002", "three": "R-003"}[request.Model]
		content := ""
		if request.ResponseFormat.JSONSchema.Name == "reviewer" {
			hash := regexp.MustCompile(`sha256=([a-f0-9]{64})`).FindStringSubmatch(request.Messages[1].Content)
			if len(hash) != 2 {
				t.Errorf("packet hash missing from prompt")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			content = fmt.Sprintf(`{"schema_version":1,"reviewer_id":%q,"findings":[{"schema_version":2,"id":%q,"reviewer":%q,"origin":"panel","severity":"major","category":"correctness","anchor":{"kind":"quote","packet_item":"artifact:brief.md","quote":"launch date is unsupported","item_sha256":%q},"issue":"The date lacks support.","recommendation":"Cite a source or remove the date.","evidence_status":"anchored","confidence":"high"}]}`, reviewer, "F-"+reviewer, reviewer, hash[1])
		} else {
			content = fmt.Sprintf(`{"schema_version":1,"votes":[{"schema_version":1,"reviewer_id":%q,"finding_id":"F-R-001","choice":"accept","severity":"major","reason":"The claim needs support."}]}`, reviewer)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}))
	defer server.Close()
	configText := fmt.Sprintf("schema_version = 1\n[openai_compatible]\nbase_url = %q\n", server.URL)
	if err := os.WriteFile(filepath.Join(documentDir, ".tribunal.toml"), []byte(configText), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadDir(documentDir)
	stateRoot := t.TempDir()
	t.Setenv("PATH", "")
	root := NewRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"--json", "--state-root", stateRoot, "--trust-workspace-config", "--panel", "openai-compatible/one,openai-compatible/two,openai-compatible/three", "review", documentPath})
	err := root.Execute()
	var exit *app.ExitError
	if !errors.As(err, &exit) || exit.Code != app.ExitBlockingFindings {
		t.Fatalf("Execute() error = %v, output = %s", err, output.String())
	}
	var final map[string]any
	if err := json.Unmarshal(output.Bytes(), &final); err != nil {
		t.Fatalf("output is not stable JSON: %v\n%s", err, output.String())
	}
	if final["status"] != "findings" {
		t.Fatalf("status = %v, want findings", final["status"])
	}
	after, _ := os.ReadDir(documentDir)
	if len(after) != len(before) {
		t.Fatalf("review wrote into document workspace: before=%v after=%v", names(before), names(after))
	}
	content, _ := os.ReadFile(documentPath)
	if string(content) != original {
		t.Fatalf("review changed source: %q", content)
	}
}

func TestRootContainsOnlyTribunalCommands(t *testing.T) {
	root := NewRootCommand()
	allowed := "adopt arbitrate bench decisions doctor edit explain findings persona recommend replay resume revert review status transcript tui verify-install version"
	var names []string
	for _, command := range root.Commands() {
		names = append(names, command.Name())
	}
	if strings.Join(names, " ") != allowed {
		t.Fatalf("commands = %q", strings.Join(names, " "))
	}
}

func names(entries []os.DirEntry) []string {
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Name())
	}
	return result
}
