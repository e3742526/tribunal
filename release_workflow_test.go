package main

import (
	"os"
	"strings"
	"testing"
)

func TestReleaseSmokePrecedesPublication(t *testing.T) {
	raw, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(raw)
	build := strings.Index(workflow, "  build-release:")
	smoke := strings.Index(workflow, "  archive-smoke:")
	publish := strings.Index(workflow, "  publish:")
	create := strings.Index(workflow, "gh release create")
	if build < 0 || smoke <= build || publish <= smoke || create <= publish {
		t.Fatalf("release jobs are not ordered build -> smoke -> publish")
	}
	for _, required := range []string{"args: release --clean --skip=publish", "needs: build-release", "needs: archive-smoke", "if-no-files-found: error"} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow missing %q", required)
		}
	}
	if strings.Contains(workflow, "archive-smoke:\n    name:") && strings.Contains(workflow[smoke:publish], "needs: publish") {
		t.Fatal("archive smoke still depends on publication")
	}
}
