package adapters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

const ReviewSchema = `{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "type":"object","additionalProperties":false,
  "required":["schema_version","reviewer_id","findings"],
  "properties":{
    "schema_version":{"const":1},"reviewer_id":{"type":"string"},"summary":{"type":"string"},
    "findings":{"type":"array","maxItems":25,"items":{"$ref":"#/$defs/finding"}}
  },
  "$defs":{"finding":{"type":"object","additionalProperties":false,
    "required":["schema_version","id","reviewer","origin","severity","category","anchor","issue","recommendation","evidence_status","confidence"],
    "properties":{"schema_version":{"const":2},"id":{"type":"string"},"reviewer":{"type":"string"},"persona":{"type":"string"},"origin":{"enum":["panel","worker"]},
      "severity":{"enum":["blocker","major","minor","nit"]},"category":{"enum":["correctness","evidence","citation-integrity","factual-claim","security","data-loss","integrity","style","scope","structure"]},
      "anchor":{"type":"object","required":["kind","packet_item","quote","item_sha256"],"properties":{"kind":{"enum":["quote","section"]},"packet_item":{"type":"string"},"quote":{"type":"string"},"prefix":{"type":"string"},"suffix":{"type":"string"},"char_offset":{"type":"integer"},"end_offset":{"type":"integer"},"item_sha256":{"type":"string"}},"additionalProperties":false},
      "issue":{"type":"string"},"recommendation":{"type":"string"},"evidence":{"type":"array","items":{"type":"string"}},"evidence_status":{"enum":["anchored","worker-verified","unevidenced"]},"confidence":{"enum":["low","med","high"]},"redacted_input":{"type":"boolean"},"quarantined":{"type":"boolean"},"quarantine_reason":{"type":"string"}}
    }}
}`

const VoteSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["schema_version","votes"],"properties":{"schema_version":{"const":1},"votes":{"type":"array","items":{"type":"object","required":["schema_version","reviewer_id","finding_id","choice","severity","reason"],"properties":{"schema_version":{"const":1},"reviewer_id":{"type":"string"},"finding_id":{"type":"string"},"choice":{"enum":["accept","reject","modify","abstain"]},"severity":{"enum":["blocker","major","minor","nit"]},"reason":{"type":"string"},"modification":{"type":"string"}},"additionalProperties":false}}},"additionalProperties":false}`

const EditSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":false,"required":["schema_version","run_id","packet_hash","hunks"],"properties":{"schema_version":{"const":1},"run_id":{"type":"string"},"packet_hash":{"type":"string"},"hunks":{"type":"array","minItems":1,"items":{"type":"object","additionalProperties":false,"required":["packet_item","finding_ids","scope","source_sha256","start","end","replacement"],"properties":{"packet_item":{"type":"string"},"finding_ids":{"type":"array","minItems":1,"items":{"type":"string"}},"scope":{"enum":["local","section","document"]},"source_sha256":{"type":"string"},"start":{"type":"integer","minimum":0},"end":{"type":"integer","minimum":0},"replacement":{"type":"string"}}}}}}`

func DecodeReview(raw []byte, reviewer string) (domain.Review, bool, error) {
	var review domain.Review
	repaired, err := decodeContract(raw, ReviewSchema, &review)
	if err != nil {
		return domain.Review{}, repaired, err
	}
	if review.SchemaVersion != domain.SchemaVersion || review.ReviewerID == "" {
		return domain.Review{}, repaired, fmt.Errorf("review requires explicit schema_version and reviewer_id")
	}
	if reviewer != "" && review.ReviewerID != reviewer {
		return domain.Review{}, repaired, fmt.Errorf("reviewer_id %q does not match %q", review.ReviewerID, reviewer)
	}
	seen := map[string]bool{}
	for i := range review.Findings {
		if err := domain.ValidateFinding(review.Findings[i]); err != nil {
			return domain.Review{}, repaired, err
		}
		if review.Findings[i].Reviewer != review.ReviewerID || review.Findings[i].Origin != "panel" {
			return domain.Review{}, repaired, fmt.Errorf("finding %q has invalid reviewer or origin binding", review.Findings[i].ID)
		}
		if seen[review.Findings[i].ID] {
			return domain.Review{}, repaired, fmt.Errorf("duplicate finding id %q", review.Findings[i].ID)
		}
		seen[review.Findings[i].ID] = true
	}
	return review, repaired, nil
}

func DecodeVotes(raw []byte, reviewer string) ([]domain.Vote, bool, error) {
	var payload struct {
		SchemaVersion int           `json:"schema_version"`
		Votes         []domain.Vote `json:"votes"`
	}
	repaired, err := decodeContract(raw, VoteSchema, &payload)
	if err != nil {
		return nil, repaired, err
	}
	seen := map[string]bool{}
	for _, vote := range payload.Votes {
		if vote.ReviewerID != reviewer {
			return nil, repaired, fmt.Errorf("vote reviewer_id %q does not match %q", vote.ReviewerID, reviewer)
		}
		if seen[vote.FindingID] {
			return nil, repaired, fmt.Errorf("reviewer %q voted more than once on %q", reviewer, vote.FindingID)
		}
		seen[vote.FindingID] = true
	}
	return payload.Votes, repaired, nil
}

func DecodeEdit(raw []byte, runID, packetHash string) (domain.EditProposal, bool, error) {
	var proposal domain.EditProposal
	repaired, err := decodeContract(raw, EditSchema, &proposal)
	if err != nil {
		return domain.EditProposal{}, repaired, err
	}
	if proposal.RunID != runID || proposal.PacketHash != packetHash {
		return domain.EditProposal{}, repaired, fmt.Errorf("edit proposal run or packet identity mismatch")
	}
	return proposal, repaired, nil
}

func DecodeEditStrict(raw []byte, runID, packetHash string) (domain.EditProposal, error) {
	var proposal domain.EditProposal
	if err := DecodeStrict(raw, EditSchema, &proposal); err != nil {
		return domain.EditProposal{}, err
	}
	if proposal.RunID != runID || proposal.PacketHash != packetHash {
		return domain.EditProposal{}, fmt.Errorf("edit proposal run or packet identity mismatch")
	}
	return proposal, nil
}

func DecodeStrict(raw []byte, schemaText string, target any) error {
	repaired, err := decodeContract(raw, schemaText, target)
	if err != nil {
		return err
	}
	if repaired {
		return fmt.Errorf("input must be exactly one JSON object")
	}
	return nil
}

func decodeContract(raw []byte, schemaText string, target any) (bool, error) {
	candidates := jsonCandidates(raw)
	var last error
	for index, candidate := range candidates {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(candidate))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			last = err
			continue
		}
		if err := ensureEOF(decoder); err != nil {
			last = err
			continue
		}
		compiler := jsonschema.NewCompiler()
		var schemaDocument any
		if err := json.Unmarshal([]byte(schemaText), &schemaDocument); err != nil {
			return false, err
		}
		if err := compiler.AddResource("schema.json", schemaDocument); err != nil {
			return false, err
		}
		schema, err := compiler.Compile("schema.json")
		if err != nil {
			return false, err
		}
		if err := schema.Validate(value); err != nil {
			last = err
			continue
		}
		if err := json.Unmarshal(candidate, target); err != nil {
			last = err
			continue
		}
		return index > 0 || !bytes.Equal(bytes.TrimSpace(raw), candidate), nil
	}
	return false, fmt.Errorf("model output contract invalid: %w", last)
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

var fencedJSON = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")

func jsonCandidates(raw []byte) [][]byte {
	trimmed := bytes.TrimSpace(raw)
	candidates := [][]byte{trimmed}
	for _, match := range fencedJSON.FindAllSubmatch(raw, 8) {
		candidates = append(candidates, bytes.TrimSpace(match[1]))
	}
	for start, count := 0, 0; start < len(raw) && count < 64 && len(candidates) < 10; start++ {
		if raw[start] != '{' {
			continue
		}
		count++
		if end := balancedObject(raw, start); end > start {
			candidates = append(candidates, bytes.TrimSpace(raw[start:end]))
		}
	}
	return candidates
}

func balancedObject(raw []byte, start int) int {
	depth := 0
	inString, escaped := false, false
	for i := start; i < len(raw); i++ {
		character := raw[i]
		if inString {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == '"' {
				inString = false
			}
			continue
		}
		if character == '"' {
			inString = true
		} else if character == '{' {
			depth++
		} else if character == '}' {
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}
