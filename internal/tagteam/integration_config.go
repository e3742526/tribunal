package tagteam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const integrationBegin = "# BEGIN tagteam managed integration"
const integrationEnd = "# END tagteam managed integration"

type IntegrationResult struct {
	Target  string `json:"target"`
	Status  string `json:"status"`
	Changed bool   `json:"changed"`
	Detail  string `json:"detail,omitempty"`
	Content []byte `json:"-"`
}

func PlanIntegration(target string, existing []byte) (IntegrationResult, error) {
	if target == "mcp-json" {
		return planJSONIntegration(target, existing)
	}
	block := []byte(integrationBegin + "\n# Tagteam MCP-compatible CLI: tagteam intel <orient|find|trace|impact|resume|recall|evidence>\n" + integrationEnd + "\n")
	original := append([]byte(nil), existing...)
	begin, end, err := integrationMarkers(existing)
	if err != nil {
		return IntegrationResult{}, err
	}
	if begin >= 0 {
		existing = append(append(append([]byte{}, existing[:begin]...), block...), existing[end:]...)
	} else {
		if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
			existing = append(existing, '\n')
		}
		existing = append(existing, block...)
	}
	return IntegrationResult{Target: target, Status: "planned", Changed: !bytes.Equal(original, existing), Detail: "only the marked Tagteam block is managed", Content: existing}, nil
}

func DoctorIntegration(target string, existing []byte) IntegrationResult {
	if target == "mcp-json" {
		var raw map[string]any
		if json.Unmarshal(existing, &raw) != nil {
			return IntegrationResult{Target: target, Status: "corrupt", Detail: "invalid JSON"}
		}
		servers, _ := raw["mcpServers"].(map[string]any)
		if _, ok := servers["tagteam"]; ok {
			return IntegrationResult{Target: target, Status: "installed"}
		}
		return IntegrationResult{Target: target, Status: "absent"}
	}
	begin, _, err := integrationMarkers(existing)
	if err != nil {
		return IntegrationResult{Target: target, Status: "corrupt", Detail: err.Error()}
	}
	if begin < 0 {
		return IntegrationResult{Target: target, Status: "absent"}
	}
	return IntegrationResult{Target: target, Status: "installed"}
}

func UninstallIntegration(target string, existing []byte) (IntegrationResult, error) {
	if target == "mcp-json" {
		var raw map[string]any
		if len(existing) == 0 {
			return IntegrationResult{Target: target, Status: "absent", Content: existing}, nil
		}
		if err := json.Unmarshal(existing, &raw); err != nil {
			return IntegrationResult{}, fmt.Errorf("invalid JSON: %w", err)
		}
		servers, _ := raw["mcpServers"].(map[string]any)
		delete(servers, "tagteam")
		data, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return IntegrationResult{}, err
		}
		data = append(data, '\n')
		return IntegrationResult{Target: target, Status: "uninstalled", Changed: !bytes.Equal(data, existing), Content: data}, nil
	}
	begin, end, err := integrationMarkers(existing)
	if err != nil {
		return IntegrationResult{}, err
	}
	if begin < 0 {
		return IntegrationResult{Target: target, Status: "absent", Content: existing}, nil
	}
	data := append(append([]byte{}, existing[:begin]...), existing[end:]...)
	return IntegrationResult{Target: target, Status: "uninstalled", Changed: true, Content: data}, nil
}

func planJSONIntegration(target string, existing []byte) (IntegrationResult, error) {
	raw := map[string]any{}
	if len(existing) > 0 && json.Unmarshal(existing, &raw) != nil {
		return IntegrationResult{}, fmt.Errorf("invalid JSON; refusing to replace user configuration")
	}
	servers, ok := raw["mcpServers"].(map[string]any)
	if !ok {
		servers = map[string]any{}
		raw["mcpServers"] = servers
	}
	servers["tagteam"] = map[string]any{"command": "tagteam", "args": []string{"intel"}}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return IntegrationResult{}, err
	}
	data = append(data, '\n')
	return IntegrationResult{Target: target, Status: "planned", Changed: !bytes.Equal(data, existing), Detail: "JSON formatting may change; unknown keys are preserved", Content: data}, nil
}

func integrationMarkers(data []byte) (int, int, error) {
	begin := bytes.Index(data, []byte(integrationBegin))
	end := bytes.Index(data, []byte(integrationEnd))
	if (begin < 0) != (end < 0) || (begin >= 0 && end < begin) {
		return -1, -1, fmt.Errorf("corrupt Tagteam integration markers")
	}
	if begin < 0 {
		return -1, -1, nil
	}
	end += len(integrationEnd)
	if end < len(data) && data[end] == '\n' {
		end++
	}
	return begin, end, nil
}
func validIntegrationTarget(target string) bool {
	return strings.Contains(" codex claude cursor vscode mcp-json ", " "+target+" ")
}
