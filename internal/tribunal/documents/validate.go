package documents

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

// ValidatePacket re-establishes every persisted packet invariant at the trust
// boundary. Stored identity fields are never accepted as proof of their data.
func ValidatePacket(packet Packet) error {
	if packet.SchemaVersion != domain.SchemaVersion || packet.Kind == "" || packet.InputRoot == "" || packet.WorkspaceID == "" || len(packet.Items) == 0 {
		return fmt.Errorf("invalid packet schema or identity")
	}
	if !validSHA256(packet.PacketHash) || hashString(packet.Rubric) != packet.RubricHash {
		return fmt.Errorf("packet or rubric hash mismatch")
	}
	root, err := canonicalExisting(packet.InputRoot)
	if err != nil || root != filepath.Clean(packet.InputRoot) {
		return fmt.Errorf("packet input root is no longer canonical")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("packet input root is unavailable or unsafe")
	}
	workspaceRoot := root
	if !rootInfo.IsDir() {
		workspaceRoot = filepath.Dir(root)
	}
	if shortHash(workspaceRoot) != packet.WorkspaceID {
		return fmt.Errorf("packet workspace identity mismatch")
	}
	items := make(map[string]Item, len(packet.Items))
	logical := map[string]bool{}
	for _, item := range packet.Items {
		if item.SchemaVersion != domain.SchemaVersion || item.ID == "" || item.LogicalPath == "" || item.MediaType == "" || !utf8.ValidString(item.Content) {
			return fmt.Errorf("invalid packet item schema or content")
		}
		if _, exists := items[item.ID]; exists || logical[item.LogicalPath] {
			return fmt.Errorf("duplicate packet item identity")
		}
		if !validSHA256(item.SourceSHA256) || hashString(item.Content) != item.PacketSHA256 {
			return fmt.Errorf("packet item %q hash mismatch", item.ID)
		}
		if !filepath.IsAbs(item.SourcePath) || filepath.Clean(item.SourcePath) != item.SourcePath {
			return fmt.Errorf("packet item %q has a non-canonical path", item.ID)
		}
		if err := ensureStillCanonical(item.SourcePath, root, rootInfo.IsDir()); err != nil {
			return err
		}
		items[item.ID], logical[item.LogicalPath] = item, true
	}
	for _, evidence := range packet.Evidence {
		if err := domain.ValidateEvidenceItem(evidence); err != nil {
			return err
		}
	}
	for _, redaction := range packet.Redactions {
		item, ok := items[redaction.PacketItem]
		if redaction.SchemaVersion != domain.SchemaVersion || !ok || redaction.Class == "" || redaction.Reason == "" || redaction.Start < 0 || redaction.End < redaction.Start || redaction.End > len(item.Content) {
			return fmt.Errorf("invalid packet redaction")
		}
	}
	seenChunks := map[string]bool{}
	for _, chunk := range packet.Chunks {
		item, ok := items[chunk.PacketItem]
		if chunk.SchemaVersion != domain.SchemaVersion || chunk.ID == "" || seenChunks[chunk.ID] || !ok || chunk.Start < 0 || chunk.End <= chunk.Start || chunk.End > len(item.Content) || item.Content[chunk.Start:chunk.End] != chunk.Content || !utf8.ValidString(chunk.Content) {
			return fmt.Errorf("invalid packet chunk %q", chunk.ID)
		}
		seenChunks[chunk.ID] = true
	}
	want, err := canonicalPacketHash(packet)
	if err != nil || want != packet.PacketHash {
		return fmt.Errorf("canonical packet hash mismatch")
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
