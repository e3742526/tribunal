// Package domain contains Tribunal's pure review and decision model.
package domain

import "time"

const (
	SchemaVersion        = 1
	FindingSchemaVersion = 2
)

type Severity string

const (
	SeverityNit     Severity = "nit"
	SeverityMinor   Severity = "minor"
	SeverityMajor   Severity = "major"
	SeverityBlocker Severity = "blocker"
)

func (s Severity) Rank() int {
	switch s {
	case SeverityBlocker:
		return 4
	case SeverityMajor:
		return 3
	case SeverityMinor:
		return 2
	case SeverityNit:
		return 1
	default:
		return 0
	}
}

func SeverityFromRank(rank int) Severity {
	switch rank {
	case 4:
		return SeverityBlocker
	case 3:
		return SeverityMajor
	case 2:
		return SeverityMinor
	default:
		return SeverityNit
	}
}

type Category string

const (
	CategoryCorrectness       Category = "correctness"
	CategoryEvidence          Category = "evidence"
	CategoryCitationIntegrity Category = "citation-integrity"
	CategoryFactualClaim      Category = "factual-claim"
	CategorySecurity          Category = "security"
	CategoryDataLoss          Category = "data-loss"
	CategoryIntegrity         Category = "integrity"
	CategoryStyle             Category = "style"
	CategoryScope             Category = "scope"
	CategoryStructure         Category = "structure"
)

func (c Category) Strict() bool {
	return c == CategorySecurity || c == CategoryDataLoss || c == CategoryCitationIntegrity
}

type EvidenceStatus string

const (
	EvidenceAnchored       EvidenceStatus = "anchored"
	EvidenceWorkerVerified EvidenceStatus = "worker-verified"
	EvidenceUnevidenced    EvidenceStatus = "unevidenced"
)

type Anchor struct {
	Kind       string `json:"kind"`
	PacketItem string `json:"packet_item"`
	Quote      string `json:"quote,omitempty"`
	Prefix     string `json:"prefix,omitempty"`
	Suffix     string `json:"suffix,omitempty"`
	CharOffset int    `json:"char_offset,omitempty"`
	EndOffset  int    `json:"end_offset,omitempty"`
	ItemSHA256 string `json:"item_sha256"`
}

type Finding struct {
	SchemaVersion  int            `json:"schema_version"`
	ID             string         `json:"id"`
	Reviewer       string         `json:"reviewer"`
	Persona        string         `json:"persona,omitempty"`
	Origin         string         `json:"origin"`
	Severity       Severity       `json:"severity"`
	Category       Category       `json:"category"`
	Anchor         Anchor         `json:"anchor"`
	Issue          string         `json:"issue"`
	Recommendation string         `json:"recommendation"`
	Evidence       []string       `json:"evidence,omitempty"`
	EvidenceStatus EvidenceStatus `json:"evidence_status"`
	Confidence     string         `json:"confidence"`
	RedactedInput  bool           `json:"redacted_input,omitempty"`
	Quarantined    bool           `json:"quarantined,omitempty"`
	QuarantineWhy  string         `json:"quarantine_reason,omitempty"`
}

type EvidenceItem struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	Task          string    `json:"task"`
	Phase         string    `json:"phase"`
	Source        string    `json:"source"`
	Query         string    `json:"query,omitempty"`
	RetrievedAt   time.Time `json:"retrieved_at"`
	Excerpt       string    `json:"excerpt"`
	ContentSHA256 string    `json:"content_sha256"`
	Status        string    `json:"status"`
	Error         string    `json:"error,omitempty"`
}

type Panelist struct {
	ID                   string  `json:"id" toml:"id"`
	Adapter              string  `json:"adapter" toml:"adapter"`
	Model                string  `json:"model" toml:"model"`
	Family               string  `json:"family" toml:"family"`
	Persona              string  `json:"persona,omitempty" toml:"persona"`
	Weight               float64 `json:"weight" toml:"weight"`
	Trusted              bool    `json:"trusted,omitempty" toml:"trusted"`
	MaxContextTokens     int     `json:"max_context_tokens" toml:"max_context_tokens"`
	ReservedOutputTokens int     `json:"reserved_output_tokens" toml:"reserved_output_tokens"`
}

type Panel struct {
	SchemaVersion int        `json:"schema_version" toml:"schema_version"`
	Reviewers     []Panelist `json:"reviewers" toml:"reviewer"`
}

type Review struct {
	SchemaVersion int       `json:"schema_version"`
	ReviewerID    string    `json:"reviewer_id"`
	Findings      []Finding `json:"findings"`
	Summary       string    `json:"summary,omitempty"`
}

type VoteChoice string

const (
	VoteAccept  VoteChoice = "accept"
	VoteReject  VoteChoice = "reject"
	VoteModify  VoteChoice = "modify"
	VoteAbstain VoteChoice = "abstain"
)

type Vote struct {
	SchemaVersion int        `json:"schema_version"`
	ReviewerID    string     `json:"reviewer_id"`
	FindingID     string     `json:"finding_id"`
	Choice        VoteChoice `json:"choice"`
	Severity      Severity   `json:"severity"`
	Reason        string     `json:"reason"`
	Modification  string     `json:"modification,omitempty"`
}

type Cluster struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	Category      Category  `json:"category"`
	MemberIDs     []string  `json:"member_ids"`
	Anchor        Anchor    `json:"anchor"`
	Finding       Finding   `json:"finding"`
	Votes         []Vote    `json:"votes,omitempty"`
	Decision      *Decision `json:"decision,omitempty"`
}

type Dissent struct {
	ReviewerID string     `json:"reviewer_id"`
	Choice     VoteChoice `json:"choice"`
	Severity   Severity   `json:"severity"`
	Reason     string     `json:"reason"`
}

type Decision struct {
	SchemaVersion int       `json:"schema_version"`
	FindingID     string    `json:"finding_id"`
	Outcome       string    `json:"outcome"`
	Severity      Severity  `json:"severity"`
	Accepts       int       `json:"accepts"`
	Rejects       int       `json:"rejects"`
	Abstains      int       `json:"abstains"`
	Configured    int       `json:"configured_reviewers"`
	Valid         int       `json:"valid_reviewers"`
	Strict        bool      `json:"category_strict"`
	Reason        string    `json:"reason"`
	Dissent       []Dissent `json:"dissent,omitempty"`
}

type ArbitrationDispute struct {
	SchemaVersion int      `json:"schema_version"`
	ID            string   `json:"id"`
	Finding       Finding  `json:"finding"`
	Decision      Decision `json:"decision"`
	ForArgument   string   `json:"for_argument,omitempty"`
	Against       string   `json:"against_argument,omitempty"`
	Default       string   `json:"default_recommendation"`
}

type PanelStatus struct {
	ReviewerID string `json:"reviewer_id"`
	Adapter    string `json:"adapter"`
	Model      string `json:"model"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
}

type DeliveryRecord struct {
	SchemaVersion int       `json:"schema_version"`
	InvocationID  string    `json:"invocation_id"`
	ReviewerID    string    `json:"reviewer_id"`
	Adapter       string    `json:"adapter"`
	Model         string    `json:"model"`
	Phase         string    `json:"phase"`
	PacketHash    string    `json:"packet_hash"`
	Items         []string  `json:"items"`
	Chunks        []string  `json:"chunks,omitempty"`
	Truncated     bool      `json:"truncated"`
	DeliveredAt   time.Time `json:"delivered_at"`
}

type RunPhase string

const (
	PhaseInit               RunPhase = "INIT"
	PhasePacketBuilt        RunPhase = "PACKET_BUILT"
	PhaseReviewing          RunPhase = "REVIEWING"
	PhaseReviewed           RunPhase = "REVIEWED"
	PhaseVerifying          RunPhase = "VERIFYING"
	PhaseClustered          RunPhase = "CLUSTERED"
	PhaseVoting             RunPhase = "VOTING"
	PhaseConsensus          RunPhase = "CONSENSUS"
	PhaseArbitrationPending RunPhase = "ARBITRATION_PENDING"
	PhaseDegraded           RunPhase = "DEGRADED"
	PhaseRecommended        RunPhase = "RECOMMENDED"
	PhaseEditPending        RunPhase = "EDIT_PENDING"
	PhaseEdited             RunPhase = "EDITED"
	PhaseRereviewing        RunPhase = "REREVIEWING"
	PhaseFinal              RunPhase = "FINAL"
	PhaseAborted            RunPhase = "ABORTED"
)

type RunState struct {
	SchemaVersion int       `json:"schema_version"`
	RunID         string    `json:"run_id"`
	WorkspaceID   string    `json:"workspace_id"`
	PacketHash    string    `json:"packet_hash,omitempty"`
	Phase         RunPhase  `json:"phase"`
	Status        string    `json:"status"`
	ReasonCodes   []string  `json:"reason_codes,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Final struct {
	SchemaVersion   int                  `json:"schema_version"`
	RunID           string               `json:"run_id"`
	WorkspaceID     string               `json:"workspace_id"`
	PacketHash      string               `json:"packet_hash"`
	Status          string               `json:"status"`
	ExitCode        int                  `json:"exit_code"`
	Summary         string               `json:"summary"`
	PanelIncomplete bool                 `json:"panel_incomplete"`
	DegradedReason  string               `json:"degraded_reason,omitempty"`
	ReasonCodes     []string             `json:"reason_codes,omitempty"`
	PanelStatus     []PanelStatus        `json:"panel_status"`
	Findings        []Finding            `json:"findings"`
	Decisions       []Decision           `json:"decisions"`
	Arbitration     []ArbitrationDispute `json:"arbitration,omitempty"`
	EditsApplied    bool                 `json:"edits_applied"`
	StartedAt       time.Time            `json:"started_at"`
	FinishedAt      time.Time            `json:"finished_at"`
}

type EditScope string

const (
	EditLocal    EditScope = "local"
	EditSection  EditScope = "section"
	EditDocument EditScope = "document"
)

type EditHunk struct {
	PacketItem   string    `json:"packet_item"`
	FindingIDs   []string  `json:"finding_ids"`
	Scope        EditScope `json:"scope"`
	SourceSHA256 string    `json:"source_sha256"`
	Start        int       `json:"start"`
	End          int       `json:"end"`
	Replacement  string    `json:"replacement"`
}

type EditProposal struct {
	SchemaVersion int        `json:"schema_version"`
	RunID         string     `json:"run_id"`
	PacketHash    string     `json:"packet_hash"`
	Hunks         []EditHunk `json:"hunks"`
}
