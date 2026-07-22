package architecture

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Node struct {
	ID     string   `json:"id"`
	Path   string   `json:"path"`
	Owner  string   `json:"owner"`
	Kind   string   `json:"kind"`
	Allows []string `json:"allows"`
}
type Intent struct {
	SchemaVersion int      `json:"schema_version"`
	MissionRef    string   `json:"mission_ref"`
	AuthorityRefs []string `json:"authority_refs"`
	Nodes         []Node   `json:"nodes"`
}
type Invariant struct {
	ID       string   `json:"id"`
	Severity string   `json:"severity"`
	Owner    string   `json:"owner"`
	NodeIDs  []string `json:"node_ids"`
	Sources  []string `json:"sources"`
	Evidence []string `json:"evidence"`
}
type Invariants struct {
	SchemaVersion int         `json:"schema_version"`
	Invariants    []Invariant `json:"invariants"`
}
type Exception struct {
	ID          string `json:"id"`
	InvariantID string `json:"invariant_id"`
	Reason      string `json:"reason"`
	ApprovedBy  string `json:"approved_by"`
	Expires     string `json:"expires"`
}
type Exceptions struct {
	SchemaVersion int         `json:"schema_version"`
	Exceptions    []Exception `json:"exceptions"`
}
type Baseline struct {
	SchemaVersion int      `json:"schema_version"`
	RecordedAt    string   `json:"recorded_at"`
	Basis         string   `json:"basis"`
	HealthScore   int      `json:"health_score"`
	Trend         string   `json:"trend"`
	Passing       []string `json:"passing_invariants"`
	Violations    []string `json:"violations"`
}

func Validate(root string, now time.Time) error {
	var intent Intent
	if err := readStrict(filepath.Join(root, ".architecture", "intent.json"), &intent); err != nil {
		return err
	}
	var invariants Invariants
	if err := readStrict(filepath.Join(root, ".architecture", "invariants.json"), &invariants); err != nil {
		return err
	}
	var exceptions Exceptions
	if err := readStrict(filepath.Join(root, ".architecture", "exceptions.json"), &exceptions); err != nil {
		return err
	}
	var baseline Baseline
	if err := readStrict(filepath.Join(root, ".architecture", "baseline.json"), &baseline); err != nil {
		return err
	}
	if intent.SchemaVersion != 1 || invariants.SchemaVersion != 1 || exceptions.SchemaVersion != 1 || baseline.SchemaVersion != 1 {
		return fmt.Errorf("architecture artifacts require schema_version 1")
	}
	if intent.MissionRef == "" || !exists(root, intent.MissionRef) {
		return fmt.Errorf("architecture mission reference is missing")
	}
	for _, ref := range intent.AuthorityRefs {
		if !exists(root, ref) {
			return fmt.Errorf("architecture authority reference is missing: %s", ref)
		}
	}
	nodes := map[string]Node{}
	for _, node := range intent.Nodes {
		if node.ID == "" || node.Path == "" || node.Owner == "" || node.Kind == "" || nodes[node.ID].ID != "" || !exists(root, node.Path) {
			return fmt.Errorf("invalid architecture node %q", node.ID)
		}
		nodes[node.ID] = node
	}
	known := map[string]Invariant{}
	for _, invariant := range invariants.Invariants {
		if invariant.ID == "" || invariant.Owner == "" || len(invariant.NodeIDs) == 0 || len(invariant.Sources) == 0 || len(invariant.Evidence) == 0 || known[invariant.ID].ID != "" {
			return fmt.Errorf("invalid architecture invariant %q", invariant.ID)
		}
		if invariant.Severity != "high" && invariant.Severity != "medium" && invariant.Severity != "low" {
			return fmt.Errorf("invalid severity for %s", invariant.ID)
		}
		for _, id := range invariant.NodeIDs {
			if nodes[id].ID == "" {
				return fmt.Errorf("invariant %s references unknown node %s", invariant.ID, id)
			}
		}
		for _, source := range invariant.Sources {
			if !exists(root, source) {
				return fmt.Errorf("invariant %s source is missing: %s", invariant.ID, source)
			}
		}
		known[invariant.ID] = invariant
	}
	activeExceptions := map[string]bool{}
	for _, exception := range exceptions.Exceptions {
		if exception.ID == "" || known[exception.InvariantID].ID == "" || exception.Reason == "" || exception.ApprovedBy == "" {
			return fmt.Errorf("invalid architecture exception %q", exception.ID)
		}
		expires, err := time.Parse("2006-01-02", exception.Expires)
		if err != nil || expires.Before(now.Truncate(24*time.Hour)) {
			return fmt.Errorf("architecture exception %s is expired or invalid", exception.ID)
		}
		activeExceptions[exception.InvariantID] = true
	}
	accounted := map[string]bool{}
	for _, id := range baseline.Passing {
		if known[id].ID == "" || accounted[id] {
			return fmt.Errorf("invalid passing invariant %q", id)
		}
		accounted[id] = true
	}
	for _, id := range baseline.Violations {
		if known[id].ID == "" || accounted[id] || !activeExceptions[id] {
			return fmt.Errorf("unaccepted baseline violation %q", id)
		}
		accounted[id] = true
	}
	if len(accounted) != len(known) {
		return fmt.Errorf("baseline does not account for every invariant")
	}
	wantScore := 100
	if len(baseline.Violations) > 0 {
		wantScore = 100 - (100*len(baseline.Violations))/len(known)
	}
	if baseline.HealthScore != wantScore {
		return fmt.Errorf("baseline health_score=%d, want %d", baseline.HealthScore, wantScore)
	}
	return validateDependencies(root, nodes)
}

func readStrict(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode %s: trailing JSON", filepath.Base(path))
		}
		return err
	}
	return nil
}

func exists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
	return err == nil
}

func validateDependencies(root string, nodes map[string]Node) error {
	var goNodes []Node
	for _, node := range nodes {
		if node.Kind == "go-package" {
			goNodes = append(goNodes, node)
		}
	}
	sort.Slice(goNodes, func(i, j int) bool { return len(goNodes[i].Path) > len(goNodes[j].Path) })
	for _, node := range goNodes {
		allowed := map[string]bool{node.ID: true}
		for _, id := range node.Allows {
			if nodes[id].ID == "" {
				return fmt.Errorf("node %s allows unknown node %s", node.ID, id)
			}
			allowed[id] = true
		}
		err := filepath.WalkDir(filepath.Join(root, node.Path), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || strings.HasSuffix(path, "_test.go") || !strings.HasSuffix(path, ".go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, spec := range file.Imports {
				name := strings.Trim(spec.Path.Value, "\"")
				const prefix = "github.com/e3742526/tribunal/"
				if !strings.HasPrefix(name, prefix+"internal/") {
					continue
				}
				rel := strings.TrimPrefix(name, prefix)
				target := ""
				for _, candidate := range goNodes {
					if rel == candidate.Path || strings.HasPrefix(rel, candidate.Path+"/") {
						target = candidate.ID
						break
					}
				}
				if target != "" && !allowed[target] {
					return fmt.Errorf("architecture dependency violation: %s imports %s", node.ID, target)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
