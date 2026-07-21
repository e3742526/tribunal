// Package documents builds immutable, content-addressed review packets.
package documents

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

type Redaction struct {
	SchemaVersion int    `json:"schema_version"`
	PacketItem    string `json:"packet_item"`
	Start         int    `json:"start"`
	End           int    `json:"end"`
	Class         string `json:"class"`
	Reason        string `json:"reason"`
}

type Item struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	LogicalPath   string `json:"logical_path"`
	MediaType     string `json:"media_type"`
	SourcePath    string `json:"source_path"`
	SourceSHA256  string `json:"source_sha256"`
	PacketSHA256  string `json:"packet_sha256"`
	Content       string `json:"content"`
	Editable      bool   `json:"editable"`
}

type Chunk struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	PacketItem    string `json:"packet_item"`
	Start         int    `json:"start"`
	End           int    `json:"end"`
	Content       string `json:"content"`
}

type Packet struct {
	SchemaVersion int                   `json:"schema_version"`
	Kind          string                `json:"kind"`
	InputRoot     string                `json:"input_root"`
	WorkspaceID   string                `json:"workspace_id"`
	PacketHash    string                `json:"packet_hash"`
	Rubric        string                `json:"rubric"`
	RubricHash    string                `json:"rubric_hash"`
	Items         []Item                `json:"items"`
	Evidence      []domain.EvidenceItem `json:"evidence,omitempty"`
	Redactions    []Redaction           `json:"redactions,omitempty"`
	Chunks        []Chunk               `json:"chunks,omitempty"`
}

type BuildOptions struct {
	Kind             string
	Rubric           string
	FailOnSecret     bool
	KnownSecrets     []string
	PDFTimeout       time.Duration
	MaxExtractedByte int64
}

// WorkspaceIdentity returns the stable external-state identity and canonical
// document root used by Build without extracting document content.
func WorkspaceIdentity(input string) (string, string, error) {
	canonical, err := canonicalExisting(input)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", "", err
	}
	root := canonical
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return "", "", fmt.Errorf("input must be a regular file or directory")
		}
		root = filepath.Dir(canonical)
	}
	return shortHash(root), root, nil
}

func Build(ctx context.Context, input string, opts BuildOptions) (Packet, error) {
	if opts.Kind == "" {
		opts.Kind = "generic"
	}
	if opts.PDFTimeout <= 0 {
		opts.PDFTimeout = 2 * time.Minute
	}
	if opts.MaxExtractedByte <= 0 {
		opts.MaxExtractedByte = 16 << 20
	}
	canonical, err := canonicalExisting(input)
	if err != nil {
		return Packet{}, err
	}
	info, err := os.Lstat(canonical)
	if err != nil {
		return Packet{}, fmt.Errorf("inspect input: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Packet{}, fmt.Errorf("input may not be a symlink")
	}
	workspaceRoot := canonical
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return Packet{}, fmt.Errorf("input must be a regular file or directory")
		}
		workspaceRoot = filepath.Dir(canonical)
	}
	paths, err := selectedPaths(canonical, info)
	if err != nil {
		return Packet{}, err
	}
	if len(paths) == 0 {
		return Packet{}, fmt.Errorf("input contains no supported documents")
	}
	packet := Packet{
		SchemaVersion: domain.SchemaVersion,
		Kind:          opts.Kind,
		InputRoot:     canonical,
		WorkspaceID:   shortHash(workspaceRoot),
		Rubric:        opts.Rubric,
		RubricHash:    hashString(opts.Rubric),
	}
	for index, path := range paths {
		if err := ensureStillCanonical(path, canonical, info.IsDir()); err != nil {
			return Packet{}, err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return Packet{}, fmt.Errorf("read %s: %w", path, err)
		}
		content, mediaType, editable, err := extract(ctx, path, raw, opts)
		if err != nil {
			return Packet{}, err
		}
		logical := filepath.Base(path)
		if info.IsDir() {
			logical, err = filepath.Rel(canonical, path)
			if err != nil {
				return Packet{}, err
			}
			logical = filepath.ToSlash(logical)
		}
		id := "artifact:" + logical
		redacted, redactions := scanAndRedact(id, content, opts.KnownSecrets)
		if len(redactions) > 0 && opts.FailOnSecret {
			return Packet{}, fmt.Errorf("secret or PII detected in %s; refusing due to --fail-on-secret", logical)
		}
		packet.Redactions = append(packet.Redactions, redactions...)
		packet.Items = append(packet.Items, Item{
			SchemaVersion: domain.SchemaVersion,
			ID:            id,
			LogicalPath:   logical,
			MediaType:     mediaType,
			SourcePath:    path,
			SourceSHA256:  hashBytes(raw),
			PacketSHA256:  hashString(redacted),
			Content:       redacted,
			Editable:      editable,
		})
		_ = index
	}
	packet.PacketHash, err = canonicalPacketHash(packet)
	if err != nil {
		return Packet{}, err
	}
	return packet, nil
}

func selectedPaths(root string, rootInfo os.FileInfo) ([]string, error) {
	if !rootInfo.IsDir() {
		if !supported(root) {
			return nil, fmt.Errorf("unsupported document extension %q", filepath.Ext(root))
		}
		return []string{root}, nil
	}
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if supported(path) {
				return fmt.Errorf("selected document is a symlink: %s", path)
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			if supported(path) {
				return fmt.Errorf("selected document is not a regular file: %s", path)
			}
			return nil
		}
		if supported(path) {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

func supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".txt", ".docx", ".pdf":
		return true
	default:
		return false
	}
}

func extract(ctx context.Context, path string, raw []byte, opts BuildOptions) (string, string, bool, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		if !utf8.Valid(raw) {
			return "", "", false, fmt.Errorf("%s is not valid UTF-8", path)
		}
		return string(raw), "text/markdown", true, nil
	case ".txt":
		if !utf8.Valid(raw) {
			return "", "", false, fmt.Errorf("%s is not valid UTF-8", path)
		}
		return string(raw), "text/plain", true, nil
	case ".docx":
		text, err := extractDOCX(path)
		return text, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", false, err
	case ".pdf":
		text, err := extractPDF(ctx, path, opts.PDFTimeout, opts.MaxExtractedByte)
		return text, "application/pdf", false, err
	default:
		return "", "", false, fmt.Errorf("unsupported document %s", path)
	}
}

func extractDOCX(path string) (string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("open DOCX %s: %w", path, err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			return "", err
		}
		defer stream.Close()
		decoder := xml.NewDecoder(io.LimitReader(stream, 32<<20))
		var out strings.Builder
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", fmt.Errorf("parse DOCX XML: %w", err)
			}
			switch value := token.(type) {
			case xml.CharData:
				out.Write([]byte(value))
			case xml.EndElement:
				if value.Name.Local == "p" || value.Name.Local == "tr" {
					out.WriteByte('\n')
				}
			}
		}
		return strings.TrimSpace(out.String()) + "\n", nil
	}
	return "", fmt.Errorf("DOCX %s has no word/document.xml", path)
}

func extractPDF(ctx context.Context, path string, timeout time.Duration, maxBytes int64) (string, error) {
	binary, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", fmt.Errorf("PDF review requires pdftotext: %w", err)
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(callCtx, binary, "-enc", "UTF-8", "-layout", "--", path, "-")
	var stdout bytes.Buffer
	cmd.Stdout = &limitedBuffer{buffer: &stdout, remaining: maxBytes}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if callCtx.Err() != nil {
			return "", fmt.Errorf("pdftotext timed out: %w", callCtx.Err())
		}
		return "", fmt.Errorf("pdftotext failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if !utf8.Valid(stdout.Bytes()) {
		return "", fmt.Errorf("pdftotext returned invalid UTF-8")
	}
	return stdout.String(), nil
}

type limitedBuffer struct {
	buffer    *bytes.Buffer
	remaining int64
}

func (w *limitedBuffer) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		return 0, fmt.Errorf("extracted document exceeds byte cap")
	}
	w.remaining -= int64(len(data))
	return w.buffer.Write(data)
}

func canonicalExisting(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve input: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("canonicalize input: %w", err)
	}
	return filepath.Clean(canonical), nil
}

func ensureStillCanonical(path, root string, rootIsDir bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("revalidate %s: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("revalidate %s: not a canonical regular file", path)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("revalidate %s: %w", path, err)
	}
	boundary := root
	if !rootIsDir {
		boundary = filepath.Dir(root)
	}
	rel, err := filepath.Rel(boundary, canonical)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("document escapes canonical input root: %s", path)
	}
	return nil
}

func canonicalPacketHash(packet Packet) (string, error) {
	type hashItem struct {
		ID           string `json:"id"`
		LogicalPath  string `json:"logical_path"`
		PacketSHA256 string `json:"packet_sha256"`
	}
	projection := struct {
		SchemaVersion int                   `json:"schema_version"`
		Kind          string                `json:"kind"`
		RubricHash    string                `json:"rubric_hash"`
		Items         []hashItem            `json:"items"`
		Evidence      []domain.EvidenceItem `json:"evidence,omitempty"`
	}{SchemaVersion: packet.SchemaVersion, Kind: packet.Kind, RubricHash: packet.RubricHash, Evidence: packet.Evidence}
	for _, item := range packet.Items {
		projection.Items = append(projection.Items, hashItem{ID: item.ID, LogicalPath: item.LogicalPath, PacketSHA256: item.PacketSHA256})
	}
	data, err := json.Marshal(projection)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func shortHash(value string) string  { return hashString(value)[:24] }
func hashString(value string) string { return hashBytes([]byte(value)) }
func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
