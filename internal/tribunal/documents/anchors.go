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
	if anchor.ItemSHA256 != item.PacketSHA256 {
		return fmt.Errorf("anchor item hash does not match packet")
	}
	if anchor.Quote == "" {
		return fmt.Errorf("anchor quote is empty")
	}
	if at := strings.Index(item.Content, anchor.Quote); at >= 0 {
		anchor.CharOffset, anchor.EndOffset = at, at+len(anchor.Quote)
		return nil
	}
	// Bounded fuzzy resolution accepts a unique span bracketed by both context
	// strings. It never performs semantic or edit-distance guessing.
	if anchor.Prefix == "" || anchor.Suffix == "" {
		return fmt.Errorf("quote not found and bounded context is incomplete")
	}
	prefix := strings.LastIndex(item.Content, anchor.Prefix)
	if prefix < 0 {
		return fmt.Errorf("anchor prefix not found")
	}
	start := prefix + len(anchor.Prefix)
	relEnd := strings.Index(item.Content[start:], anchor.Suffix)
	if relEnd < 0 || relEnd > 4096 {
		return fmt.Errorf("anchor suffix not uniquely reachable within bound")
	}
	end := start + relEnd
	if strings.Contains(item.Content[end+len(anchor.Suffix):], anchor.Prefix) {
		return fmt.Errorf("anchor context is ambiguous")
	}
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
			packet.Chunks = append(packet.Chunks, Chunk{SchemaVersion: 1, ID: fmt.Sprintf("chunk:%04d", len(packet.Chunks)+1), PacketItem: item.ID, Start: start, End: end, Content: item.Content[start:end]})
			start = end
		}
	}
	sort.SliceStable(packet.Chunks, func(i, j int) bool { return packet.Chunks[i].ID < packet.Chunks[j].ID })
	return nil
}

func findItem(packet Packet, id string) (Item, bool) {
	for _, item := range packet.Items {
		if item.ID == id {
			return item, true
		}
	}
	return Item{}, false
}

func isUTF8Boundary(value string, index int) bool {
	return index == len(value) || index == 0 || value[index]&0xc0 != 0x80
}
