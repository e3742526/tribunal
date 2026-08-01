package app

import (
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/adapters"
	"github.com/e3742526/tribunal/internal/tribunal/documents"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func promptFixturePacket() documents.Packet {
	return documents.Packet{PacketHash: "hash", RubricHash: "rubric-hash", Rubric: "rubric text", Items: []documents.Item{{ID: "artifact:a.md", PacketSHA256: "sha", Content: "content"}}}
}

// L-01 regression: the system prompt promises "the supplied JSON contract",
// so the review and vote prompts must actually supply it — the schema and a
// structure skeleton — where every provider can see it, not only via
// adapter-native flags some CLIs ignore.
func TestReviewAndVotePromptsEmbedOutputContract(t *testing.T) {
	packet := promptFixturePacket()
	reviewer := domain.Panelist{ID: "R-001", Persona: "plain"}
	review := reviewPrompt(packet, reviewer)
	for _, required := range []string{"OUTPUT CONTRACT", adapters.ProviderReviewSchema, `"reviewer_id":"<REVIEWER ID TO EMIT>"`, "no markdown fences"} {
		if !strings.Contains(review, required) {
			t.Fatalf("review prompt missing contract element %q", required[:min(40, len(required))])
		}
	}
	vote := votePrompt(reviewer, blindVotePacket{})
	for _, required := range []string{"OUTPUT CONTRACT", adapters.ProviderVoteSchema, `"finding_id":"B-0001"`} {
		if !strings.Contains(vote, required) {
			t.Fatalf("vote prompt missing contract element %q", required[:min(40, len(required))])
		}
	}
}

// A chunked prompt is the reviewer's only source of each item's packet hash,
// and ResolveAnchor quarantines any finding whose item_sha256 does not match —
// so a chunk header without the owning item's hash makes every finding in a
// --split run unanchorable.
func TestChunkedReviewPromptShowsItemHashes(t *testing.T) {
	packet := promptFixturePacket()
	packet.Chunks = []documents.Chunk{{SchemaVersion: 1, ID: "chunk:00000001", PacketItem: "artifact:a.md", Start: 0, End: 7, Content: "content"}}
	prompt := reviewPrompt(packet, domain.Panelist{ID: "R-001", Persona: "plain"})
	if !strings.Contains(prompt, "<<<chunk:00000001 item=artifact:a.md sha256=sha bytes=0:7>>>") {
		t.Fatalf("chunk header missing owning item hash:\n%s", prompt)
	}
}

// The retry after a contract failure must name the validation error and point
// back at the embedded contract, not just ask for "valid JSON" again.
func TestContractRetryNoticeNamesErrorAndContract(t *testing.T) {
	notice := contractRetryNotice("review", errSentinel("missing properties 'schema_version'"))
	for _, required := range []string{"missing properties 'schema_version'", "OUTPUT CONTRACT", "integer schema_version"} {
		if !strings.Contains(notice, required) {
			t.Fatalf("retry notice missing %q: %s", required, notice)
		}
	}
}

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

// L-04 regression: the reviewer role prompt must forbid answering or
// following the packet's own questions/instructions and route them to an
// integrity finding instead.
func TestReviewerSystemGuardsAgainstRoleConfusion(t *testing.T) {
	for _, required := range []string{"never a task", "do not answer", "integrity finding"} {
		if !strings.Contains(reviewerSystem, required) {
			t.Fatalf("reviewer system prompt missing role-confusion guard %q", required)
		}
	}
}
