package documents

import (
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func aliasFixture() Packet {
	content := "alpha target beta"
	return Packet{Items: []Item{{ID: "artifact:C08.md", LogicalPath: "C08.md", PacketSHA256: hashString(content), Content: content}}}
}

// L-02 regression: models emit "C08.md" for packet item "artifact:C08.md";
// the alias must resolve, the anchor must be canonicalized to the real item
// ID, and the item hash binding must still be enforced.
func TestResolveAnchorAcceptsPacketItemAliases(t *testing.T) {
	packet := aliasFixture()
	for _, alias := range []string{"artifact:C08.md", "C08.md"} {
		anchor := domain.Anchor{PacketItem: alias, ItemSHA256: packet.Items[0].PacketSHA256, Quote: "target"}
		if err := ResolveAnchor(packet, &anchor); err != nil {
			t.Fatalf("alias %q failed to resolve: %v", alias, err)
		}
		if anchor.PacketItem != "artifact:C08.md" {
			t.Fatalf("alias %q not canonicalized: %q", alias, anchor.PacketItem)
		}
	}
}

func TestResolveAnchorAliasStillEnforcesItemHash(t *testing.T) {
	packet := aliasFixture()
	anchor := domain.Anchor{PacketItem: "C08.md", ItemSHA256: "wrong-hash", Quote: "target"}
	if err := ResolveAnchor(packet, &anchor); err == nil || !strings.Contains(err.Error(), "hash") {
		t.Fatalf("alias resolution bypassed the item hash binding: %v", err)
	}
}

func TestResolveAnchorUnknownItemStillFails(t *testing.T) {
	packet := aliasFixture()
	anchor := domain.Anchor{PacketItem: "other.md", ItemSHA256: packet.Items[0].PacketSHA256, Quote: "target"}
	if err := ResolveAnchor(packet, &anchor); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown packet item accepted: %v", err)
	}
}
