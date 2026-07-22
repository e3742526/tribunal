package config

import (
	"strings"
	"testing"
	"time"
)

// A sub-second call timeout truncates to zero seconds at the adapter
// boundary, which adapters read as "use the 15-minute default" — reject it.
func TestNormalizeRejectsSubSecondTimeouts(t *testing.T) {
	cfg := Default()
	cfg.Limits.CallTimeout = 500 * time.Millisecond
	if err := normalize(&cfg); err == nil || !strings.Contains(err.Error(), "at least 1s") {
		t.Fatalf("sub-second call timeout accepted: %v", err)
	}
	cfg = Default()
	cfg.Limits.RunTimeout = 900 * time.Millisecond
	if err := normalize(&cfg); err == nil {
		t.Fatal("sub-second run timeout accepted")
	}
}

// Non-positive verification/arbitration caps silently disable the pipelines
// they gate while the run still reports success — reject them.
func TestNormalizeRejectsNonPositivePipelineCaps(t *testing.T) {
	cfg := Default()
	cfg.Limits.MaxVerification = -1
	if err := normalize(&cfg); err == nil {
		t.Fatal("negative max_verification accepted")
	}
	cfg = Default()
	cfg.Limits.MaxArbitration = 0
	if err := normalize(&cfg); err == nil {
		t.Fatal("zero max_arbitration accepted")
	}
}

func TestResolvePersonaRejectsPathTraversal(t *testing.T) {
	if _, err := ResolvePersona("../../evil", "", false); err == nil || !strings.Contains(err.Error(), "invalid persona name") {
		t.Fatalf("traversal persona name accepted: %v", err)
	}
}
