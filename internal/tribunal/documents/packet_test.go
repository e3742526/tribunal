package documents

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func TestBuildPacketIsDeterministicAndRedacts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("contact a@example.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("# Claim\n\nEvidence.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	one, err := Build(context.Background(), dir, BuildOptions{Kind: "generic", Rubric: "rubric"})
	if err != nil {
		t.Fatal(err)
	}
	two, err := Build(context.Background(), dir, BuildOptions{Kind: "generic", Rubric: "rubric"})
	if err != nil {
		t.Fatal(err)
	}
	if one.PacketHash != two.PacketHash || one.Items[0].LogicalPath != "a.md" {
		t.Fatalf("packet not deterministic: %#v", one.Items)
	}
	if len(one.Redactions) != 1 || one.Items[1].Content == "contact a@example.test\n" {
		t.Fatalf("redaction missing: %#v", one.Redactions)
	}
}

func TestResolveAnchorAndChunkUTF8(t *testing.T) {
	packet := Packet{Items: []Item{{ID: "artifact:x.md", PacketSHA256: "hash", Content: "αβ\n\nquoted text\n"}}}
	anchor := domain.Anchor{PacketItem: "artifact:x.md", ItemSHA256: "hash", Quote: "quoted text"}
	if err := ResolveAnchor(packet, &anchor); err != nil {
		t.Fatal(err)
	}
	if anchor.CharOffset <= 0 || anchor.EndOffset <= anchor.CharOffset {
		t.Fatalf("anchor unresolved: %#v", anchor)
	}
	if err := Split(&packet, 6); err != nil {
		t.Fatal(err)
	}
	for _, chunk := range packet.Chunks {
		if chunk.Content == "" || !utf8.ValidString(chunk.Content) {
			t.Fatalf("invalid chunk %q", chunk.Content)
		}
	}
}

func TestExtractDOCX(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.docx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(`<w:document xmlns:w="x"><w:body><w:p><w:r><w:t>Hello</w:t></w:r></w:p></w:body></w:document>`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	text, err := extractDOCX(path)
	if err != nil || text != "Hello\n" {
		t.Fatalf("text=%q err=%v", text, err)
	}
}
