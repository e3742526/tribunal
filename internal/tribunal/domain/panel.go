package domain

import (
	"fmt"
	"regexp"
	"strings"
)

var personaSlug = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

func ParsePanel(raw string) (Panel, error) {
	parts := strings.Split(raw, ",")
	panel := Panel{SchemaVersion: SchemaVersion}
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return Panel{}, fmt.Errorf("panel entry %d is empty", index+1)
		}
		slash := strings.IndexByte(part, '/')
		if slash < 1 || slash == len(part)-1 {
			return Panel{}, fmt.Errorf("panel entry %q must be adapter/model[@persona]", part)
		}
		adapter, rest := part[:slash], part[slash+1:]
		persona := "plain"
		if at := strings.LastIndexByte(rest, '@'); at >= 0 {
			candidate := rest[at+1:]
			if !personaSlug.MatchString(candidate) {
				return Panel{}, fmt.Errorf("panel persona %q is not a valid slug", candidate)
			}
			persona, rest = candidate, rest[:at]
		}
		if strings.TrimSpace(rest) == "" {
			return Panel{}, fmt.Errorf("panel entry %q has an empty model", part)
		}
		panel.Reviewers = append(panel.Reviewers, Panelist{
			ID:                   fmt.Sprintf("R-%03d", index+1),
			Adapter:              adapter,
			Model:                rest,
			Family:               adapter,
			Persona:              persona,
			Weight:               1,
			MaxContextTokens:     131072,
			ReservedOutputTokens: 16384,
		})
	}
	if len(panel.Reviewers) == 0 {
		return Panel{}, fmt.Errorf("panel requires at least one reviewer")
	}
	return panel, nil
}

func ValidatePanel(panel Panel) error {
	if panel.SchemaVersion != SchemaVersion {
		return fmt.Errorf("panel schema_version must be %d", SchemaVersion)
	}
	seen := map[string]struct{}{}
	for i := range panel.Reviewers {
		r := &panel.Reviewers[i]
		if r.ID == "" || r.Adapter == "" || r.Model == "" || r.Family == "" {
			return fmt.Errorf("reviewer %d requires id, adapter, model, and family", i+1)
		}
		if _, exists := seen[r.ID]; exists {
			return fmt.Errorf("duplicate reviewer id %q", r.ID)
		}
		seen[r.ID] = struct{}{}
		if r.Weight < 0.5 {
			r.Weight = 0.5
		}
		if r.Weight > 2 {
			r.Weight = 2
		}
		if r.MaxContextTokens <= r.ReservedOutputTokens || r.ReservedOutputTokens < 0 {
			return fmt.Errorf("reviewer %q has invalid context budget", r.ID)
		}
		if r.Persona != "" && !personaSlug.MatchString(r.Persona) {
			return fmt.Errorf("reviewer %q has invalid persona", r.ID)
		}
	}
	return nil
}

func DiversityNote(panel Panel) string {
	families := map[string]int{}
	for _, reviewer := range panel.Reviewers {
		families[reviewer.Family]++
	}
	if len(families) == 1 {
		return "single-family panel: agreement is strongly correlated"
	}
	for family, count := range families {
		if count > 1 {
			return fmt.Sprintf("%d of %d reviewers share the %s family; treat agreement as correlated", count, len(panel.Reviewers), family)
		}
	}
	return fmt.Sprintf("%d reviewers from %d distinct families", len(panel.Reviewers), len(families))
}
