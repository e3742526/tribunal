package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyInstallationRejectsDevUnlessExplicit(t *testing.T) {
	oldVersion, oldCommit, oldTime, oldDirty := Version, CommitSHA, BuildTime, Dirty
	t.Cleanup(func() { Version, CommitSHA, BuildTime, Dirty = oldVersion, oldCommit, oldTime, oldDirty })
	Version, CommitSHA, BuildTime, Dirty = "dev", "", "", "unknown"
	if _, err := verifyInstallation(false); err == nil {
		t.Fatal("expected development build rejection")
	}
	report, err := verifyInstallation(true)
	if err != nil || report.Status != "dev_build" || !report.AllowedDev {
		t.Fatalf("report=%#v err=%v", report, err)
	}
}

func TestChecksumManifestNamesAdjacentBinary(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "tagteam")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	hash, err := hashFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	manifest := binary + ".sha256"
	if err := os.WriteFile(manifest, []byte(hash+"  tagteam\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readChecksumManifest(manifest, "tagteam")
	if err != nil || got != hash {
		t.Fatalf("checksum=%q err=%v", got, err)
	}
	if _, err := readChecksumManifest(manifest, "different"); err == nil || !strings.Contains(err.Error(), "names") {
		t.Fatalf("expected filename mismatch, got %v", err)
	}
}

func TestReviewCommandRejectsUnverifiedBuild(t *testing.T) {
	oldVersion, oldCommit, oldTime, oldDirty := Version, CommitSHA, BuildTime, Dirty
	t.Cleanup(func() { Version, CommitSHA, BuildTime, Dirty = oldVersion, oldCommit, oldTime, oldDirty })
	Version, CommitSHA, BuildTime, Dirty = "dev", "", "", "unknown"
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"review"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "development build") {
		t.Fatalf("review error = %v, want installation-integrity rejection", err)
	}
}
