package tagteam

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const repositoryPointerName = "repo.json"

// RepositoryPointer is the only runtime state Tagteam needs to retain inside
// an agent worktree. All authoritative run artifacts live below StateRoot.
type RepositoryPointer struct {
	SchemaVersion int       `json:"schema_version"`
	RepoID        string    `json:"repo_id"`
	StateRoot     string    `json:"state_root"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// StateLocator centralizes every path used to discover or persist run state.
type StateLocator struct {
	Workdir     string
	RepoID      string
	StateRoot   string
	RepoRoot    string
	RunsRoot    string
	PointerPath string
	LegacyRoot  string
}

func defaultStateRoot() (string, error) {
	if value := strings.TrimSpace(os.Getenv("TAGTEAM_STATE_ROOT")); value != "" {
		return canonicalPath(value, false)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for state root: %w", err)
	}
	return filepath.Join(home, ".local", "state", "tagteam"), nil
}

func canonicalPath(path string, mustExist bool) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	if mustExist || !os.IsNotExist(err) {
		return "", err
	}
	parent, parentErr := filepath.EvalSymlinks(filepath.Dir(abs))
	if parentErr == nil {
		return filepath.Join(parent, filepath.Base(abs)), nil
	}
	return abs, nil
}

func gitCommonDirectory(workdir string) (string, error) {
	out, err := runCommand(context.Background(), workdir, "git", "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	common := strings.TrimSpace(out)
	if !filepath.IsAbs(common) {
		common = filepath.Join(workdir, common)
	}
	return canonicalPath(common, true)
}

func deriveRepoID(workdir string) (string, error) {
	common, err := gitCommonDirectory(workdir)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(common))
	return hex.EncodeToString(sum[:])[:24], nil
}

func pointerPath(workdir string) string {
	return filepath.Join(workdir, ".tagteam", repositoryPointerName)
}

func readRepositoryPointer(workdir string) (RepositoryPointer, error) {
	var pointer RepositoryPointer
	data, err := os.ReadFile(pointerPath(workdir))
	if err != nil {
		return RepositoryPointer{}, err
	}
	if err := json.Unmarshal(data, &pointer); err != nil {
		return RepositoryPointer{}, err
	}
	if pointer.SchemaVersion != ArtifactSchemaVersion || strings.TrimSpace(pointer.RepoID) == "" || strings.TrimSpace(pointer.StateRoot) == "" {
		return RepositoryPointer{}, fmt.Errorf("invalid repository pointer")
	}
	return pointer, nil
}

func resolveStateLocator(workdir, override string) (StateLocator, error) {
	canonicalWorkdir, err := canonicalPath(workdir, true)
	if err != nil {
		return StateLocator{}, err
	}
	repoID, err := deriveRepoID(canonicalWorkdir)
	if err != nil {
		return StateLocator{}, err
	}
	root := strings.TrimSpace(override)
	if root == "" {
		if pointer, readErr := readRepositoryPointer(canonicalWorkdir); readErr == nil && pointer.RepoID == repoID {
			root = pointer.StateRoot
		}
	}
	if root == "" {
		root, err = defaultStateRoot()
		if err != nil {
			return StateLocator{}, err
		}
	} else {
		root, err = canonicalPath(root, false)
		if err != nil {
			return StateLocator{}, err
		}
	}
	repoRoot := filepath.Join(root, repoID)
	return StateLocator{
		Workdir:     canonicalWorkdir,
		RepoID:      repoID,
		StateRoot:   root,
		RepoRoot:    repoRoot,
		RunsRoot:    filepath.Join(repoRoot, "runs"),
		PointerPath: pointerPath(canonicalWorkdir),
		LegacyRoot:  filepath.Join(canonicalWorkdir, ".tagteam"),
	}, nil
}

func locatorFromPointer(workdir string) (StateLocator, error) {
	pointer, err := readRepositoryPointer(workdir)
	if err != nil {
		return StateLocator{}, err
	}
	canonicalWorkdir, err := canonicalPath(workdir, true)
	if err != nil {
		return StateLocator{}, err
	}
	repoRoot := filepath.Join(pointer.StateRoot, pointer.RepoID)
	return StateLocator{
		Workdir:     canonicalWorkdir,
		RepoID:      pointer.RepoID,
		StateRoot:   pointer.StateRoot,
		RepoRoot:    repoRoot,
		RunsRoot:    filepath.Join(repoRoot, "runs"),
		PointerPath: pointerPath(canonicalWorkdir),
		LegacyRoot:  filepath.Join(canonicalWorkdir, ".tagteam"),
	}, nil
}

func (l StateLocator) Prepare() error {
	if err := os.MkdirAll(l.RunsRoot, 0o700); err != nil {
		return err
	}
	if err := l.migrateLegacyRuntimeState(); err != nil {
		return err
	}
	pointer := RepositoryPointer{
		SchemaVersion: ArtifactSchemaVersion,
		RepoID:        l.RepoID,
		StateRoot:     l.StateRoot,
		UpdatedAt:     time.Now().UTC(),
	}
	return writeJSONDurable(l.PointerPath, pointer, true, false)
}

func (l StateLocator) RunDir(runID string) (string, error) {
	if err := validateRunID(runID); err != nil {
		return "", err
	}
	return filepath.Join(l.RunsRoot, runID), nil
}

func validateRunID(runID string) error {
	if runID == "" || filepath.Base(runID) != runID || runID == "." || runID == ".." || strings.ContainsAny(runID, `/\\`) {
		return fmt.Errorf("invalid run id %q", runID)
	}
	return nil
}

func (l StateLocator) migrateLegacyRuntimeState() error {
	legacyPaths := []string{
		filepath.Join(l.LegacyRoot, "runs"),
		filepath.Join(l.LegacyRoot, "active.json"),
		filepath.Join(l.LegacyRoot, "latest.json"),
	}
	for _, source := range legacyPaths {
		if _, err := os.Lstat(source); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		destination := filepath.Join(l.RepoRoot, filepath.Base(source))
		if err := copyAndVerifyPath(source, destination); err != nil {
			return fmt.Errorf("migrate legacy state %s: %w", source, err)
		}
		if err := os.RemoveAll(source); err != nil {
			return fmt.Errorf("remove verified legacy state %s: %w", source, err)
		}
	}
	legacyIgnore := filepath.Join(l.LegacyRoot, ".gitignore")
	if data, err := os.ReadFile(legacyIgnore); err == nil && string(data) == "*\n!.gitignore\n" {
		_ = os.Remove(legacyIgnore)
	}
	return nil
}

func copyAndVerifyPath(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to migrate symlink %s", source)
	}
	if info.IsDir() {
		if err := os.MkdirAll(destination, 0o700); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if err := copyAndVerifyPath(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to migrate non-regular file %s", source)
	}
	if existing, err := os.ReadFile(destination); err == nil {
		sourceData, readErr := os.ReadFile(source)
		if readErr != nil {
			return readErr
		}
		if sha256.Sum256(existing) != sha256.Sum256(sourceData) {
			return fmt.Errorf("destination already exists with different content: %s", destination)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".migrate-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		return err
	}
	sourceHash, err := hashFile(source)
	if err != nil {
		return err
	}
	destinationHash, err := hashFile(destination)
	if err != nil {
		return err
	}
	if sourceHash != destinationHash {
		return fmt.Errorf("checksum mismatch after copying %s", source)
	}
	return syncDirectory(filepath.Dir(destination))
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

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func statePathForWorkdir(workdir, name string) string {
	if locator, err := locatorFromPointer(workdir); err == nil {
		return filepath.Join(locator.RepoRoot, name)
	}
	return filepath.Join(workdir, ".tagteam", name)
}

func runDirForWorkdir(workdir, runID string) (string, error) {
	if locator, err := locatorFromPointer(workdir); err == nil {
		return locator.RunDir(runID)
	}
	if err := validateRunID(runID); err != nil {
		return "", err
	}
	return filepath.Join(workdir, ".tagteam", "runs", runID), nil
}
