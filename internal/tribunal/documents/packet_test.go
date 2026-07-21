package documents

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestBuildRejectsSelectedSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	if err := os.WriteFile(target, []byte("text"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Build(context.Background(), link, BuildOptions{Kind: "generic", Rubric: "rubric"}); err == nil {
		t.Fatal("expected symlink input rejection")
	}
}

func TestSplitChangesPacketIdentity(t *testing.T) {
	packet := Packet{SchemaVersion: 1, Kind: "generic", RubricHash: "rubric", Items: []Item{{SchemaVersion: 1, ID: "artifact:x.md", LogicalPath: "x.md", MediaType: "text/markdown", SourceSHA256: "source", PacketSHA256: "packet", Content: "one two three four", Editable: true}}}
	before, err := canonicalPacketHash(packet)
	if err != nil {
		t.Fatal(err)
	}
	packet.PacketHash = before
	if err := Split(&packet, 5); err != nil {
		t.Fatal(err)
	}
	if packet.PacketHash == before {
		t.Fatal("chunk map was not bound into packet identity")
	}
}

func TestExtractPDFWhenPopplerAvailable(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext unavailable")
	}
	path := filepath.Join(t.TempDir(), "sample.pdf")
	if err := os.WriteFile(path, minimalPDF("Hello Tribunal"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, err := extractPDF(context.Background(), path, 5*time.Second, 1<<20)
	if err != nil || !strings.Contains(text, "Hello Tribunal") {
		t.Fatalf("extractPDF() = %q, %v", text, err)
	}
}

func minimalPDF(text string) []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET\n", text)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return out.Bytes()
}
