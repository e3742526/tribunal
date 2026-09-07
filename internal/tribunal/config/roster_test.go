package config

import (
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/sharedcatalog"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

// TestDefaultPanelIsRosterBacked keeps the shipped panel and the vendored
// fleet roster from drifting apart. A default naming a model the roster does
// not carry — or an adapter the roster does not let hold a review seat —
// would ship a panel the rest of the fleet does not recognize.
func TestDefaultPanelIsRosterBacked(t *testing.T) {
	panel, err := domain.ParsePanel(DefaultPanel)
	if err != nil {
		t.Fatalf("default panel does not parse: %v", err)
	}
	if len(panel.Reviewers) < 3 {
		t.Fatalf("default panel has %d reviewers, want at least 3 for cross-family diversity", len(panel.Reviewers))
	}
	families := map[string]bool{}
	for _, reviewer := range panel.Reviewers {
		if _, ok := sharedcatalog.LookupAdapter(reviewer.Adapter); !ok {
			t.Errorf("default panel names adapter %q, which the shared roster does not carry", reviewer.Adapter)
		}
		if _, ok := sharedcatalog.LookupModel(reviewer.Model); !ok {
			t.Errorf("default panel names model %q, which the shared roster does not carry", reviewer.Model)
		}
		families[reviewer.Family] = true
	}
	if len(families) < 3 {
		t.Errorf("default panel spans %d adapter families, want 3", len(families))
	}
	if strings.Contains(DefaultPanel, "(") {
		t.Error("default panel must name provider model IDs, not display names")
	}
}
