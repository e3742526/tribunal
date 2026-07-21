package adapters

import (
	"strings"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func TestSubprocessReviewerArgvIsReadOnly(t *testing.T) {
	panelist := domain.Panelist{Model: "model"}
	for _, id := range []string{"codex", "claude", "agy"} {
		for _, role := range []Role{RoleReviewer, RoleVoter, RoleEditor} {
			if id == "claude" && role == RoleEditor {
				continue
			}
			a := &Subprocess{AdapterID: id, Binary: id}
			argv, _, err := a.argv(role, panelist, Request{RunDir: "/run", Schema: ReviewSchema, SchemaPath: "/run/schema.json", OutputPath: "/run/output.json", TimeoutSeconds: 3}, "prompt")
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(argv, " ")
			if strings.Contains(joined, "workspace-write") || strings.Contains(joined, "acceptEdits") || strings.Contains(joined, "dangerously-skip") {
				t.Fatalf("%s %s argv grants mutation: %s", id, role, joined)
			}
		}
	}
}

func TestDecodeReviewUsesBoundedRecovery(t *testing.T) {
	raw := []byte("prose before\n```json\n{\"schema_version\":1,\"reviewer_id\":\"R-001\",\"findings\":[]}\n```\n")
	review, repaired, err := DecodeReview(raw, "R-001")
	if err != nil {
		t.Fatal(err)
	}
	if !repaired || review.ReviewerID != "R-001" {
		t.Fatalf("review=%#v repaired=%v", review, repaired)
	}
}

func TestDecodeReviewRejectsWrongReviewer(t *testing.T) {
	raw := []byte(`{"schema_version":1,"reviewer_id":"peer","findings":[]}`)
	if _, _, err := DecodeReview(raw, "R-001"); err == nil {
		t.Fatal("expected reviewer binding failure")
	}
}

func TestDecodeVotesRejectsDuplicateBallots(t *testing.T) {
	raw := []byte(`{"schema_version":1,"votes":[{"schema_version":1,"reviewer_id":"R-001","finding_id":"F-1","choice":"accept","severity":"major","reason":"yes"},{"schema_version":1,"reviewer_id":"R-001","finding_id":"F-1","choice":"reject","severity":"major","reason":"no"}]}`)
	if _, _, err := DecodeVotes(raw, "R-001"); err == nil {
		t.Fatal("expected duplicate vote rejection")
	}
}

func TestBoundedBufferRecordsOverflow(t *testing.T) {
	buffer := newBoundedBuffer(3)
	if n, err := buffer.Write([]byte("abcdef")); err != nil || n != 6 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if string(buffer.Bytes()) != "abc" || !buffer.Exceeded() {
		t.Fatalf("buffer=%q exceeded=%v", buffer.Bytes(), buffer.Exceeded())
	}
}

func TestUnwrapClaudeStructuredOutput(t *testing.T) {
	raw := []byte(`{"type":"result","structured_output":{"schema_version":1,"reviewer_id":"R-001","findings":[]}}`)
	want := `{"schema_version":1,"reviewer_id":"R-001","findings":[]}`
	if got := string(unwrapClaude(raw)); got != want {
		t.Fatalf("unwrapClaude() = %s, want %s", got, want)
	}
}
