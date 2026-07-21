// Package storage persists Tribunal state outside reviewed workspaces.
package storage

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

type Store struct {
	Root      string
	Clock     func() time.Time
	Entropy   io.Reader
	mu        sync.Mutex
	monotonic io.Reader
}

type Workspace struct {
	ID      string
	Root    string
	RunsDir string
}

func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", "tribunal"), nil
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		var err error
		root, err = DefaultRoot()
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve state root: %w", err)
	}
	return &Store{Root: filepath.Clean(abs), Clock: time.Now, Entropy: rand.Reader}, nil
}

func (s *Store) Workspace(id, documentRoot string) (Workspace, error) {
	if len(id) != 24 || strings.ContainsAny(id, `/\`) {
		return Workspace{}, fmt.Errorf("invalid workspace id %q", id)
	}
	stateRoot, err := canonicalParent(s.Root)
	if err != nil {
		return Workspace{}, err
	}
	docRoot, err := filepath.EvalSymlinks(documentRoot)
	if err != nil {
		return Workspace{}, fmt.Errorf("canonicalize document root: %w", err)
	}
	if containsPath(docRoot, stateRoot) || containsPath(stateRoot, docRoot) {
		return Workspace{}, fmt.Errorf("state root must be outside the document root")
	}
	root := filepath.Join(s.Root, id)
	if err := os.MkdirAll(filepath.Join(root, "runs"), 0o700); err != nil {
		return Workspace{}, fmt.Errorf("create workspace state: %w", err)
	}
	if err := revalidateBelow(s.Root, root); err != nil {
		return Workspace{}, err
	}
	return Workspace{ID: id, Root: root, RunsDir: filepath.Join(root, "runs")}, nil
}

func (s *Store) CreateRun(workspace Workspace) (string, string, error) {
	if s.Clock == nil || s.Entropy == nil {
		return "", "", fmt.Errorf("store clock and entropy are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.monotonic == nil {
		s.monotonic = ulid.Monotonic(s.Entropy, 1)
	}
	id, err := ulid.New(ulid.Timestamp(s.Clock().UTC()), s.monotonic)
	if err != nil {
		return "", "", fmt.Errorf("generate run ULID: %w", err)
	}
	runID := id.String()
	runDir := filepath.Join(workspace.RunsDir, runID)
	if err := revalidateBelow(workspace.Root, workspace.RunsDir); err != nil {
		return "", "", err
	}
	if err := os.Mkdir(runDir, 0o700); err != nil {
		return "", "", fmt.Errorf("create run directory: %w", err)
	}
	return runID, runDir, nil
}

func ValidateRunDir(workspace Workspace, runDir string) error {
	expected := filepath.Join(workspace.RunsDir, filepath.Base(runDir))
	if filepath.Clean(runDir) != expected {
		return fmt.Errorf("run directory is outside the workspace runs root")
	}
	info, err := os.Lstat(runDir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("run directory is not a canonical directory")
	}
	return revalidateBelow(workspace.Root, runDir)
}

func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	data = append(data, '\n')
	return atomicWrite(path, data, 0o600)
}

func WriteFile(path string, data []byte) error { return atomicWrite(path, data, 0o600) }

func WriteFileMode(path string, data []byte, mode os.FileMode) error {
	return atomicWrite(path, data, mode.Perm())
}

func ReadJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".tribunal-write-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	remove := true
	defer func() {
		_ = temp.Close()
		if remove {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(mode); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	remove = false
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func appendJSONLine(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing non-regular JSONL target %s", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func canonicalParent(path string) (string, error) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", fmt.Errorf("create state root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("canonicalize state root: %w", err)
	}
	return canonical, nil
}

func revalidateBelow(root, path string) error {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("revalidate state root: %w", err)
	}
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("revalidate state path: %w", err)
	}
	if !containsPath(canonicalRoot, canonicalPath) {
		return fmt.Errorf("state path escapes trusted root")
	}
	return nil
}

func containsPath(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func StateForFinal(final domain.Final) domain.RunState {
	phase := domain.PhaseFinal
	if final.ExitCode == 2 {
		phase = domain.PhaseArbitrationPending
	} else if final.ExitCode == 3 {
		phase = domain.PhaseDegraded
	} else if final.ExitCode == 6 {
		phase = domain.PhaseAborted
	}
	return domain.RunState{SchemaVersion: domain.SchemaVersion, RunID: final.RunID, WorkspaceID: final.WorkspaceID, PacketHash: final.PacketHash, Phase: phase, Status: final.Status, ReasonCodes: final.ReasonCodes, UpdatedAt: final.FinishedAt}
}
