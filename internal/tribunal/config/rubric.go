package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

var rubrics = map[string]string{
	"generic":    `Review for correctness, internal consistency, evidence quality, scope, structure, integrity, and clarity. Distinguish factual defects from preferences.`,
	"manuscript": `Review claims, methods, statistics, citations, limitations, reproducibility, structure, and prose. Require evidence for factual-claim findings.`,
	"strategy":   `Review objectives, assumptions, causal logic, alternatives, decision rights, resources, measures, risks, and internal consistency.`,
	"governance": `Review authority, accountability, due process, data-loss safeguards, enforcement, exceptions, auditability, ambiguity, and policy conflicts.`,
}

func BuiltinRubric(kind string) (string, bool) {
	rubric, ok := rubrics[kind]
	return rubric, ok
}

type Persona struct {
	SchemaVersion int      `toml:"schema_version" json:"schema_version"`
	Name          string   `toml:"name" json:"name"`
	Summary       string   `toml:"summary" json:"summary"`
	Focus         []string `toml:"focus" json:"focus"`
	Questions     []string `toml:"questions" json:"questions"`
	StyleNotes    []string `toml:"style_notes" json:"style_notes"`
	AllowFreeform bool     `toml:"allow_freeform" json:"allow_freeform"`
	Lens          string   `toml:"lens" json:"lens,omitempty"`
}

var forbiddenPersona = regexp.MustCompile(`(?i)(vote\s+(accept|reject)|severity\s*[:=]|ignore (the |all )?(system|role|schema)|use (a )?tool|run (a )?command|other reviewer|change your permissions)`)

func LoadPersona(path string, workspace bool) (Persona, error) {
	var persona Persona
	if _, err := toml.DecodeFile(path, &persona); err != nil {
		return Persona{}, fmt.Errorf("load persona: %w", err)
	}
	if err := LintPersona(persona, workspace); err != nil {
		return Persona{}, err
	}
	return persona, nil
}

func LintPersona(persona Persona, workspace bool) error {
	if persona.SchemaVersion != domain.SchemaVersion {
		return fmt.Errorf("persona schema_version must be %d", domain.SchemaVersion)
	}
	if !regexp.MustCompile(`^[a-z0-9-]{1,64}$`).MatchString(persona.Name) || persona.Summary == "" {
		return fmt.Errorf("persona requires a slug name and summary")
	}
	if workspace && (persona.AllowFreeform || persona.Lens != "") {
		return fmt.Errorf("workspace personas must be structured; freeform lens rejected")
	}
	if persona.Lens != "" && !persona.AllowFreeform {
		return fmt.Errorf("freeform lens requires allow_freeform=true")
	}
	text := strings.Join(append(append(append([]string{persona.Summary, persona.Lens}, persona.Focus...), persona.Questions...), persona.StyleNotes...), "\n")
	if forbiddenPersona.MatchString(text) {
		return fmt.Errorf("persona contains role, vote, schema, or permission directives")
	}
	return nil
}

func PersonaDirectories(workspace string) ([]string, error) {
	user, err := userConfigPath()
	if err != nil {
		return nil, err
	}
	dirs := []string{filepath.Join(filepath.Dir(user), "personas")}
	if workspace != "" {
		dirs = append(dirs, filepath.Join(workspace, ".tribunal", "personas"))
	}
	return dirs, nil
}

func NewPersona(path, name string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("refusing to overwrite existing persona %s", path)
	}
	if !regexp.MustCompile(`^[a-z0-9-]{1,64}$`).MatchString(name) {
		return fmt.Errorf("invalid persona name %q", name)
	}
	content := fmt.Sprintf("schema_version = 1\nname = %q\nsummary = %q\nfocus = []\nquestions = []\nstyle_notes = []\n", name, "Describe the review lens.")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
