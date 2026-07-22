package documents

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func anchorItem(content string) Packet {
	return Packet{Items: []Item{{ID: "artifact:x.md", PacketSHA256: hashString(content), Content: content}}}
}

// A quote that appears more than once must not silently bind to the first
// occurrence: anchors define the edit window, so binding must be provably
// unique or fail.
func TestResolveAnchorRejectsAmbiguousQuote(t *testing.T) {
	packet := anchorItem("alpha target beta target gamma")
	anchor := domain.Anchor{PacketItem: "artifact:x.md", ItemSHA256: packet.Items[0].PacketSHA256, Quote: "target"}
	if err := ResolveAnchor(packet, &anchor); err == nil {
		t.Fatal("ambiguous quote resolved silently")
	}
}

func TestResolveAnchorDisambiguatesWithContext(t *testing.T) {
	packet := anchorItem("alpha target beta target gamma")
	anchor := domain.Anchor{PacketItem: "artifact:x.md", ItemSHA256: packet.Items[0].PacketSHA256, Quote: "target", Prefix: "beta ", Suffix: " gamma"}
	if err := ResolveAnchor(packet, &anchor); err != nil {
		t.Fatalf("contextual disambiguation failed: %v", err)
	}
	want := strings.Index(packet.Items[0].Content, "beta ") + len("beta ")
	if anchor.CharOffset != want || anchor.EndOffset != want+len("target") {
		t.Fatalf("anchor bound to %d..%d, want %d..%d", anchor.CharOffset, anchor.EndOffset, want, want+len("target"))
	}
}

// The fuzzy path's old ambiguity check compared text after the LAST prefix
// occurrence, which can never contain the prefix — repeated prefixes were
// accepted silently.
func TestResolveAnchorFuzzyRejectsRepeatedPrefix(t *testing.T) {
	packet := anchorItem("pre middle suf ... pre other suf")
	anchor := domain.Anchor{PacketItem: "artifact:x.md", ItemSHA256: packet.Items[0].PacketSHA256, Quote: "absent", Prefix: "pre ", Suffix: " suf"}
	if err := ResolveAnchor(packet, &anchor); err == nil {
		t.Fatalf("repeated prefix accepted; bound %d..%d %q", anchor.CharOffset, anchor.EndOffset, anchor.Quote)
	}
	unique := anchorItem("pre middle suf and nothing else")
	anchor = domain.Anchor{PacketItem: "artifact:x.md", ItemSHA256: unique.Items[0].PacketSHA256, Quote: "absent", Prefix: "pre ", Suffix: " suf"}
	if err := ResolveAnchor(unique, &anchor); err != nil || anchor.Quote != "middle" {
		t.Fatalf("unique fuzzy resolution failed: %q, %v", anchor.Quote, err)
	}
}

func TestExtractDOCXRejectsDuplicateEntries(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, body := range []string{"benign", "payload"} {
		entry, err := writer.CreateRaw(&zip.FileHeader{Name: "word/document.xml"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := extractDOCX("dup.docx", buffer.Bytes(), 32<<20); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate entries accepted: %v", err)
	}
}

func TestExtractTextEnforcesCap(t *testing.T) {
	if _, err := extractText("big.md", bytes.Repeat([]byte("a"), 65), 64); err == nil {
		t.Fatal("oversized text accepted")
	}
	if text, err := extractText("ok.md", []byte("fine"), 64); err != nil || text != "fine" {
		t.Fatalf("small text rejected: %q, %v", text, err)
	}
}

func TestSplitChunkIDsSortNumerically(t *testing.T) {
	content := ""
	for i := 0; i < 12; i++ {
		content += fmt.Sprintf("paragraph %02d filler text\n\n", i)
	}
	packet := Packet{SchemaVersion: 1, Kind: "generic", RubricHash: "rubric", Items: []Item{{SchemaVersion: 1, ID: "artifact:x.md", LogicalPath: "x.md", MediaType: "text/markdown", SourceSHA256: "s", PacketSHA256: "p", Content: content, Editable: true}}}
	if err := Split(&packet, 30); err != nil {
		t.Fatal(err)
	}
	if len(packet.Chunks) < 10 {
		t.Fatalf("expected many chunks, got %d", len(packet.Chunks))
	}
	previousStart := -1
	for i, chunk := range packet.Chunks {
		if want := fmt.Sprintf("chunk:%08d", i+1); chunk.ID != want {
			t.Fatalf("chunk %d ID = %q, want %q", i, chunk.ID, want)
		}
		if chunk.Start < previousStart {
			t.Fatalf("chunk order does not follow document order")
		}
		previousStart = chunk.Start
	}
}
