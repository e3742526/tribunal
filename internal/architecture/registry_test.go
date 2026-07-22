package architecture

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRepositoryRegistryConforms(t *testing.T) {
	root := filepath.Join("..", "..")
	if err := Validate(root, time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryRejectsExpiredException(t *testing.T) {
	root := t.TempDir()
	architectureDir := filepath.Join(root, ".architecture")
	if err := os.MkdirAll(architectureDir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"intent.json":     `{"schema_version":1,"mission_ref":"mission","authority_refs":[],"nodes":[{"id":"repo","path":".","owner":"owner","kind":"repository","allows":[]}]}`,
		"invariants.json": `{"schema_version":1,"invariants":[{"id":"rule","severity":"high","owner":"owner","node_ids":["repo"],"sources":["mission"],"evidence":["test"]}]}`,
		"exceptions.json": `{"schema_version":1,"exceptions":[{"id":"EX-1","invariant_id":"rule","reason":"temporary","approved_by":"owner","expires":"2026-07-20"}]}`,
		"baseline.json":   `{"schema_version":1,"recorded_at":"2026-07-21","basis":"fixture","health_score":0,"trend":"bootstrap","passing_invariants":[],"violations":["rule"]}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(architectureDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "mission"), []byte("intent"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root, time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expired exception accepted")
	}
}

func TestDependencyValidatorRejectsForbiddenEdge(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"internal/a", "internal/b"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	source := `package a
import _ "github.com/e3742526/tribunal/internal/b"
`
	if err := os.WriteFile(filepath.Join(root, "internal/a/a.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	nodes := map[string]Node{
		"a": {ID: "a", Path: "internal/a", Kind: "go-package"},
		"b": {ID: "b", Path: "internal/b", Kind: "go-package"},
	}
	if err := validateDependencies(root, nodes); err == nil {
		t.Fatal("forbidden dependency accepted")
	}
}
