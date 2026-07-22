package cli

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/e3742526/tribunal/internal/tribunal/app"
)

var (
	CommitSHA = ""
	BuildTime = ""
	Dirty     = "unknown"
)

type InstallationReport struct {
	SchemaVersion  int    `json:"schema_version"`
	Version        string `json:"version"`
	CommitSHA      string `json:"commit_sha"`
	BuildTime      string `json:"build_time"`
	Dirty          string `json:"dirty"`
	Executable     string `json:"executable,omitempty"`
	Manifest       string `json:"manifest,omitempty"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
	ActualSHA256   string `json:"actual_sha256,omitempty"`
	Status         string `json:"status"`
	AllowedDev     bool   `json:"allowed_dev,omitempty"`
}

func newVersionCommand(f *flags) *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print Tribunal build identity", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		report := InstallationReport{SchemaVersion: 1, Version: Version, CommitSHA: CommitSHA, BuildTime: BuildTime, Dirty: Dirty, Status: "identity"}
		return printValue(cmd, f, report, Version)
	}}
}

func newVerifyInstallCommand(f *flags) *cobra.Command {
	var allowDev bool
	cmd := &cobra.Command{Use: "verify-install", Short: "Verify embedded provenance and adjacent binary checksum", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		report, err := verifyInstallation(allowDev)
		if printErr := printValue(cmd, f, report, fmt.Sprintf("status=%s version=%s executable=%s", report.Status, report.Version, report.Executable)); printErr != nil {
			return printErr
		}
		if err != nil {
			return &app.ExitError{Code: app.ExitPreflight, Err: err}
		}
		return nil
	}}
	cmd.Flags().BoolVar(&allowDev, "allow-dev-build", false, "explicitly accept an unverified development build")
	return cmd
}

func verifyInstallation(allowDev bool) (InstallationReport, error) {
	report := InstallationReport{SchemaVersion: 1, Version: Version, CommitSHA: CommitSHA, BuildTime: BuildTime, Dirty: Dirty, Status: "invalid", AllowedDev: allowDev}
	executable, err := os.Executable()
	if err != nil {
		return report, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return report, err
	}
	report.Executable, report.Manifest = executable, executable+".sha256"
	if strings.Contains(Version, "dev") || CommitSHA == "" || BuildTime == "" || Dirty == "unknown" || Dirty == "true" {
		report.Status = "dev_build"
		if allowDev {
			return report, nil
		}
		return report, fmt.Errorf("unverified development build; pass --allow-dev-build explicitly")
	}
	expected, err := readChecksumManifest(report.Manifest, filepath.Base(executable))
	if err != nil {
		report.Status = "manifest_missing_or_invalid"
		return report, err
	}
	report.ExpectedSHA256 = expected
	actual, err := hashFile(executable)
	if err != nil {
		return report, err
	}
	report.ActualSHA256 = actual
	if actual != expected {
		report.Status = "checksum_mismatch"
		return report, fmt.Errorf("installed binary checksum does not match manifest")
	}
	report.Status = "verified"
	return report, nil
}

func readChecksumManifest(path, binaryName string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, 4096))
	if !scanner.Scan() {
		return "", fmt.Errorf("installation manifest is empty")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 1 || len(fields[0]) != 64 {
		return "", fmt.Errorf("installation manifest has invalid SHA-256")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", fmt.Errorf("installation manifest has invalid SHA-256")
	}
	if len(fields) > 1 && strings.TrimPrefix(fields[1], "*") != binaryName {
		return "", fmt.Errorf("installation manifest names the wrong binary")
	}
	return strings.ToLower(fields[0]), nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
