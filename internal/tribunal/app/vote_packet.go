package app

import (
	"github.com/e3742526/tribunal/internal/tribunal/documents"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

type verificationArtifact struct {
	SchemaVersion    int                   `json:"schema_version"`
	Evidence         []domain.EvidenceItem `json:"evidence"`
	VerificationHash string                `json:"verification_hash"`
}

type votePacketItem struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	LogicalPath   string `json:"logical_path"`
	MediaType     string `json:"media_type"`
	PacketSHA256  string `json:"packet_sha256"`
	Content       string `json:"content,omitempty"`
}

type blindVotePacket struct {
	SchemaVersion        int                   `json:"schema_version"`
	PacketHash           string                `json:"packet_hash"`
	Rubric               string                `json:"rubric"`
	RubricHash           string                `json:"rubric_hash"`
	Items                []votePacketItem      `json:"items"`
	Chunks               []documents.Chunk     `json:"chunks,omitempty"`
	PreReviewEvidence    []domain.EvidenceItem `json:"pre_review_evidence,omitempty"`
	VerificationEvidence []domain.EvidenceItem `json:"verification_evidence,omitempty"`
	VerificationHash     string                `json:"verification_hash"`
	ShuffleSeed          string                `json:"shuffle_seed"`
	Findings             []domain.Finding      `json:"findings"`
}

func buildBlindVotePacket(packet documents.Packet, verification verificationArtifact, findings []domain.Finding) blindVotePacket {
	ballot := blindVotePacket{
		SchemaVersion:        1,
		PacketHash:           packet.PacketHash,
		Rubric:               packet.Rubric,
		RubricHash:           packet.RubricHash,
		Chunks:               append([]documents.Chunk(nil), packet.Chunks...),
		PreReviewEvidence:    append([]domain.EvidenceItem(nil), packet.Evidence...),
		VerificationEvidence: append([]domain.EvidenceItem(nil), verification.Evidence...),
		VerificationHash:     verification.VerificationHash,
		ShuffleSeed:          packet.PacketHash,
		Findings:             findings,
	}
	includeItemContent := len(packet.Chunks) == 0
	for _, item := range packet.Items {
		content := ""
		if includeItemContent {
			content = item.Content
		}
		ballot.Items = append(ballot.Items, votePacketItem{SchemaVersion: 1, ID: item.ID, LogicalPath: item.LogicalPath, MediaType: item.MediaType, PacketSHA256: item.PacketSHA256, Content: content})
	}
	return ballot
}
