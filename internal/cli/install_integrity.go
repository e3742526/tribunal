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

	"github.com/cephalopod-ai/tagteam/internal/tagteam"
)

var (
	CommitSHA = ""
	BuildTime = ""
	Dirty     = "unknown"
)

type InstallationReport struct {
	Version        string `json:"version"`
	CommitSHA      string `json:"commit_sha"`
	BuildTime      string `json:"build_time"`
	Dirty          string `json:"dirty"`
	Executable     string `json:"executable"`
	Manifest       string `json:"manifest"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
	ActualSHA256   string `json:"actual_sha256,omitempty"`
	Status         string `json:"status"`
	AllowedDev     bool   `json:"allowed_dev,omitempty"`
}

func verifyInstallation(allowDev bool) (InstallationReport, error) {
	report := InstallationReport{Version: Version, CommitSHA: CommitSHA, BuildTime: BuildTime, Dirty: Dirty, Status: "invalid", AllowedDev: allowDev}
	executable, err := os.Executable()
	if err != nil {
		return report, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return report, err
	}
	report.Executable = executable
	report.Manifest = executable + ".sha256"
	if Version == "dev" || strings.TrimSpace(CommitSHA) == "" || strings.TrimSpace(BuildTime) == "" || Dirty == "unknown" || Dirty == "true" {
		report.Status = "dev_build"
		if allowDev {
			return report, nil
		}
		return report, fmt.Errorf("unverified development build; pass --allow-dev-build explicitly")
	}
	expected, manifestErr := readChecksumManifest(report.Manifest, filepath.Base(executable))
	if manifestErr != nil {
		report.Status = "manifest_missing_or_invalid"
		return report, manifestErr
	}
	report.ExpectedSHA256 = expected
	actual, hashErr := hashFile(executable)
	if hashErr != nil {
		return report, hashErr
	}
	report.ActualSHA256 = actual
	if actual != expected {
		report.Status = "checksum_mismatch"
		return report, fmt.Errorf("installed binary checksum does not match %s", report.Manifest)
	}
	report.Status = "verified"
	return report, nil
}

func requireVerifiedInstallation(flags *flagState) error {
	_, err := verifyInstallation(flags.AllowDevBuild)
	if err == nil {
		return nil
	}
	return &tagteam.ExitError{Code: tagteam.ExitPreflightFailed, Err: err}
}

func readChecksumManifest(path, binaryName string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read installation manifest: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, 4096))
	if !scanner.Scan() {
		return "", fmt.Errorf("installation manifest is empty")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 1 || len(fields[0]) != 64 {
		return "", fmt.Errorf("installation manifest has an invalid SHA-256")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", fmt.Errorf("installation manifest has an invalid SHA-256")
	}
	if len(fields) > 1 && strings.TrimPrefix(fields[1], "*") != binaryName {
		return "", fmt.Errorf("installation manifest names %q, expected %q", fields[1], binaryName)
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
