package documents

import (
	"fmt"
	"sort"
	"strings"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func ResolveAnchor(packet Packet, anchor *domain.Anchor) error {
	item, ok := findItem(packet, anchor.PacketItem)
	if !ok {
		return fmt.Errorf("packet item %q not found", anchor.PacketItem)
	}
	// Canonicalize alias spellings to the item's real ID so clustering,
	// ledger scoping, and edit windows all key on one identity.
	anchor.PacketItem = item.ID
	if anchor.ItemSHA256 != item.PacketSHA256 {
		return fmt.Errorf("anchor item hash does not match packet")
	}
	if anchor.Quote == "" {
		return fmt.Errorf("anchor quote is empty")
	}
	// Anchors define edit windows, so binding must be unambiguous: a repeated
	// quote resolves only when prefix+quote+suffix isolates one occurrence.
	if at := strings.Index(item.Content, anchor.Quote); at >= 0 {
		if strings.Index(item.Content[at+1:], anchor.Quote) < 0 {
			anchor.CharOffset, anchor.EndOffset = at, at+len(anchor.Quote)
			return nil
		}
		if anchor.Prefix == "" || anchor.Suffix == "" {
			return fmt.Errorf("anchor quote occurs more than once and lacks disambiguating context")
		}
		composite := anchor.Prefix + anchor.Quote + anchor.Suffix
		at = strings.Index(item.Content, composite)
		if at < 0 || strings.Index(item.Content[at+1:], composite) >= 0 {
			return fmt.Errorf("anchor quote occurs more than once and context does not isolate one occurrence")
		}
		anchor.CharOffset = at + len(anchor.Prefix)
		anchor.EndOffset = anchor.CharOffset + len(anchor.Quote)
		return nil
	}
	// Bounded fuzzy resolution accepts a unique span bracketed by both context
	// strings. It never performs semantic or edit-distance guessing.
	if anchor.Prefix == "" || anchor.Suffix == "" {
		return fmt.Errorf("quote not found and bounded context is incomplete")
	}
	prefix := strings.Index(item.Content, anchor.Prefix)
	if prefix < 0 {
		return fmt.Errorf("anchor prefix not found")
	}
	if strings.Index(item.Content[prefix+1:], anchor.Prefix) >= 0 {
		return fmt.Errorf("anchor context is ambiguous")
	}
	start := prefix + len(anchor.Prefix)
	relEnd := strings.Index(item.Content[start:], anchor.Suffix)
	if relEnd < 0 || relEnd > 4096 {
		return fmt.Errorf("anchor suffix not uniquely reachable within bound")
	}
	end := start + relEnd
	anchor.Quote = item.Content[start:end]
	anchor.CharOffset, anchor.EndOffset = start, end
	return nil
}

func MarkRedactedOverlap(packet Packet, finding *domain.Finding) {
	for _, redaction := range packet.Redactions {
		if redaction.PacketItem == finding.Anchor.PacketItem && finding.Anchor.CharOffset < redaction.End && redaction.Start < finding.Anchor.EndOffset {
			finding.RedactedInput = true
			return
		}
	}
}

func Split(packet *Packet, maxBytes int) error {
	if maxBytes <= 0 {
		return fmt.Errorf("split byte budget must be positive")
	}
	packet.Chunks = nil
	for _, item := range packet.Items {
		start := 0
		for start < len(item.Content) {
			end := start + maxBytes
			if end >= len(item.Content) {
				end = len(item.Content)
			} else {
				for end > start && !isUTF8Boundary(item.Content, end) {
					end--
				}
				if boundary := strings.LastIndex(item.Content[start:end], "\n\n"); boundary > maxBytes/2 {
					end = start + boundary + 2
				}
			}
			if end <= start {
				return fmt.Errorf("could not create a UTF-8-safe chunk")
			}
			// Eight digits keep lexicographic ID order equal to numeric
			// order well past any realistic chunk count.
			packet.Chunks = append(packet.Chunks, Chunk{SchemaVersion: 1, ID: fmt.Sprintf("chunk:%08d", len(packet.Chunks)+1), PacketItem: item.ID, Start: start, End: end, Content: item.Content[start:end]})
			start = end
		}
	}
	sort.SliceStable(packet.Chunks, func(i, j int) bool { return packet.Chunks[i].ID < packet.Chunks[j].ID })
	hash, err := canonicalPacketHash(*packet)
	if err != nil {
		return err
	}
	packet.PacketHash = hash
	return nil
}

// findItem resolves a packet item by its canonical ID, then by the two
// unambiguous alias spellings models emit in practice (live playtest L-02:
// "C08.md" for "artifact:C08.md" quarantined otherwise-valid findings and
// silently erased multi-model agreement). Aliases never weaken integrity:
// the caller still verifies the anchor's item_sha256 against the resolved
// item, and each alias form maps to at most one item because logical paths
// are unique within a packet.
func findItem(packet Packet, id string) (Item, bool) {
	for _, item := range packet.Items {
		if item.ID == id {
			return item, true
		}
	}
	for _, item := range packet.Items {
		if item.ID == "artifact:"+id {
			return item, true
		}
	}
	for _, item := range packet.Items {
		if item.LogicalPath != "" && item.LogicalPath == id {
			return item, true
		}
	}
	return Item{}, false
}

func isUTF8Boundary(value string, index int) bool {
	return index == len(value) || index == 0 || value[index]&0xc0 != 0x80
}
