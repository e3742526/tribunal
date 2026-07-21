package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/e3742526/tribunal/internal/tribunal/adapters"
	"github.com/e3742526/tribunal/internal/tribunal/documents"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

type EditOptions struct {
	RunRef
	ProposalPath    string
	Apply           bool
	ConfirmDocument bool
	Rereview        bool
}

type EditFileRecord struct {
	PacketItem   string `json:"packet_item"`
	SourcePath   string `json:"source_path"`
	BackupPath   string `json:"backup_path"`
	BeforeSHA256 string `json:"before_sha256"`
	AfterSHA256  string `json:"after_sha256"`
}

type EditRecord struct {
	SchemaVersion int              `json:"schema_version"`
	RunID         string           `json:"run_id"`
	PacketHash    string           `json:"packet_hash"`
	Files         []EditFileRecord `json:"files"`
	AppliedAt     time.Time        `json:"applied_at"`
	RevertedAt    *time.Time       `json:"reverted_at,omitempty"`
}

type EditResult struct {
	SchemaVersion int                 `json:"schema_version"`
	RunID         string              `json:"run_id"`
	Proposal      domain.EditProposal `json:"proposal"`
	Applied       bool                `json:"applied"`
	Record        *EditRecord         `json:"record,omitempty"`
	Rereview      *domain.Final       `json:"rereview,omitempty"`
}

type plannedEdit struct {
	item  documents.Item
	live  []byte
	mode  os.FileMode
	hunks []domain.EditHunk
	after []byte
}

func (s *Service) Edit(ctx context.Context, opts EditOptions) (EditResult, error) {
	workspace, runDir, runID, err := s.locateRun(opts.RunRef)
	if err != nil {
		return EditResult{}, exitError(ExitPreflight, "%v", err)
	}
	lock, err := storage.AcquireLock(ctx, filepath.Join(runDir, "run.lock"), nil)
	if err != nil {
		return EditResult{}, exitError(ExitPreflight, "acquire run lock: %v", err)
	}
	defer lock.Close()
	if err := storage.ValidateRunDir(workspace, runDir); err != nil {
		return EditResult{}, exitError(ExitPreflight, "revalidate run: %v", err)
	}
	packet, err := readPacket(filepath.Join(runDir, "packet.json"))
	if err != nil {
		return EditResult{}, exitError(ExitPreflight, "%v", err)
	}
	final, err := readFinal(filepath.Join(runDir, "final.json"))
	if err != nil {
		return EditResult{}, exitError(ExitPreflight, "%v", err)
	}
	proposal, err := s.loadOrCreateProposal(ctx, runDir, runID, packet, final, opts.ProposalPath)
	if err != nil {
		return EditResult{}, exitError(ExitPreflight, "%v", err)
	}
	if err := storage.WriteJSON(filepath.Join(runDir, "edit-proposal.json"), proposal); err != nil {
		return EditResult{}, exitError(ExitPreflight, "persist edit proposal: %v", err)
	}
	plans, err := validateEditProposal(packet, final, proposal, opts.ConfirmDocument)
	if err != nil {
		return EditResult{}, exitError(ExitInvalidArguments, "%v", err)
	}
	result := EditResult{SchemaVersion: 1, RunID: runID, Proposal: proposal}
	if !opts.Apply {
		return result, nil
	}
	record, err := applyPlans(runDir, runID, packet.PacketHash, plans, s.now())
	if err != nil {
		return EditResult{}, exitError(ExitAborted, "apply edits: %v", err)
	}
	result.Applied, result.Record = true, &record
	final.EditsApplied = true
	final.FinishedAt = s.now()
	if err := s.transition(runDir, runID, packet, domain.PhaseEdited, "edited", nil); err != nil {
		return result, exitError(ExitAborted, "%v", err)
	}
	meta, err := readMeta(filepath.Join(runDir, "meta.json"))
	if err != nil {
		return result, exitError(ExitAborted, "%v", err)
	}
	if err := s.publish(runDir, workspace, final, meta.Panel); err != nil {
		return result, exitError(ExitAborted, "%v", err)
	}
	if opts.Rereview {
		rereview, reviewErr := s.Review(ctx, ReviewOptions{Input: packet.InputRoot, Kind: packet.Kind, PanelValue: &meta.Panel})
		result.Rereview = &rereview
		return result, reviewErr
	}
	return result, nil
}

func (s *Service) Revert(ref RunRef) (EditRecord, error) {
	workspace, runDir, _, err := s.locateRun(ref)
	if err != nil {
		return EditRecord{}, exitError(ExitPreflight, "%v", err)
	}
	lock, err := storage.AcquireLock(context.Background(), filepath.Join(runDir, "run.lock"), nil)
	if err != nil {
		return EditRecord{}, exitError(ExitPreflight, "acquire run lock: %v", err)
	}
	defer lock.Close()
	if err := storage.ValidateRunDir(workspace, runDir); err != nil {
		return EditRecord{}, exitError(ExitPreflight, "revalidate run: %v", err)
	}
	var record EditRecord
	path := filepath.Join(runDir, "edit-record.json")
	if err := storage.ReadJSON(path, &record); err != nil {
		return EditRecord{}, exitError(ExitPreflight, "load edit record: %v", err)
	}
	if record.SchemaVersion != 1 || record.RevertedAt != nil {
		return EditRecord{}, exitError(ExitInvalidArguments, "edit record is unsupported or already reverted")
	}
	type restore struct {
		file EditFileRecord
		data []byte
		live []byte
		mode os.FileMode
	}
	var restores []restore
	for _, file := range record.Files {
		canonical, err := filepath.EvalSymlinks(file.SourcePath)
		if err != nil || canonical != file.SourcePath {
			return EditRecord{}, exitError(ExitAborted, "source path changed since edit: %s", file.SourcePath)
		}
		live, err := os.ReadFile(file.SourcePath)
		if err != nil || hashText(string(live)) != file.AfterSHA256 {
			return EditRecord{}, exitError(ExitAborted, "refusing to overwrite user changes in %s", file.SourcePath)
		}
		backup, err := os.ReadFile(file.BackupPath)
		if err != nil || hashText(string(backup)) != file.BeforeSHA256 {
			return EditRecord{}, exitError(ExitAborted, "backup validation failed for %s", file.SourcePath)
		}
		info, err := os.Stat(file.SourcePath)
		if err != nil {
			return EditRecord{}, exitError(ExitAborted, "%v", err)
		}
		restores = append(restores, restore{file: file, data: backup, live: live, mode: info.Mode()})
	}
	var restored []restore
	for _, item := range restores {
		if err := storage.WriteFileMode(item.file.SourcePath, item.data, item.mode); err != nil {
			for _, prior := range restored {
				_ = storage.WriteFileMode(prior.file.SourcePath, prior.live, prior.mode)
			}
			return EditRecord{}, exitError(ExitAborted, "restore %s: %v", item.file.SourcePath, err)
		}
		restored = append(restored, item)
	}
	now := s.now()
	record.RevertedAt = &now
	if err := storage.WriteJSON(path, record); err != nil {
		return EditRecord{}, exitError(ExitAborted, "persist revert record: %v", err)
	}
	final, err := readFinal(filepath.Join(runDir, "final.json"))
	if err != nil {
		return EditRecord{}, exitError(ExitPreflight, "%v", err)
	}
	meta, err := readMeta(filepath.Join(runDir, "meta.json"))
	if err != nil {
		return EditRecord{}, exitError(ExitPreflight, "%v", err)
	}
	final.EditsApplied, final.FinishedAt = false, now
	if err := s.publish(runDir, workspace, final, meta.Panel); err != nil {
		return EditRecord{}, exitError(ExitPreflight, "persist reverted final: %v", err)
	}
	return record, nil
}

func (s *Service) loadOrCreateProposal(ctx context.Context, runDir, runID string, packet documents.Packet, final domain.Final, path string) (domain.EditProposal, error) {
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return domain.EditProposal{}, err
		}
		return adapters.DecodeEditStrict(raw, runID, packet.PacketHash)
	}
	meta, err := readMeta(filepath.Join(runDir, "meta.json"))
	if err != nil || len(meta.Panel.Reviewers) == 0 {
		return domain.EditProposal{}, fmt.Errorf("run has no editor-capable panel")
	}
	var editor domain.Panelist
	for _, candidate := range meta.Panel.Reviewers {
		if candidate.Adapter != "claude" {
			editor = candidate
			break
		}
	}
	if editor.ID == "" {
		return domain.EditProposal{}, fmt.Errorf("run panel has no proposal-capable editor")
	}
	adapter, err := s.Registry.Get(editor.Adapter)
	if err != nil {
		return domain.EditProposal{}, err
	}
	dir := filepath.Join(runDir, "calls", editor.ID, "edit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return domain.EditProposal{}, err
	}
	if err := storage.WriteFile(filepath.Join(runDir, "edit.schema.json"), []byte(adapters.EditSchema+"\n")); err != nil {
		return domain.EditProposal{}, err
	}
	prompt := editPrompt(packet, final, runID)
	if err := storage.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(prompt)); err != nil {
		return domain.EditProposal{}, err
	}
	req := adapters.Request{RunDir: dir, SystemPrompt: "Propose only typed replacement hunks for accepted Tribunal findings. Never access or modify files.", Prompt: prompt, Schema: adapters.EditSchema, SchemaPath: filepath.Join(runDir, "edit.schema.json"), OutputPath: filepath.Join(dir, "output.json"), MaxOutputBytes: s.Config.Limits.MaxOutputBytes, TimeoutSeconds: int(s.Config.Limits.CallTimeout.Seconds()), EnvSecrets: trustedSecrets(s.Config)}
	response, err := s.invokeWithProviderLock(ctx, runDir, adapter, adapters.RoleEditor, editor, req)
	if err != nil {
		return domain.EditProposal{}, err
	}
	if err := storage.WriteFile(filepath.Join(dir, "raw.json"), response.Raw); err != nil {
		return domain.EditProposal{}, err
	}
	proposal, _, err := adapters.DecodeEdit(response.Raw, runID, packet.PacketHash)
	return proposal, err
}

func editPrompt(packet documents.Packet, final domain.Final, runID string) string {
	accepted := []domain.Finding{}
	acceptedIDs := map[string]bool{}
	for _, decision := range final.Decisions {
		acceptedIDs[decision.FindingID] = decision.Outcome == "accepted"
	}
	for _, finding := range final.Findings {
		if acceptedIDs[finding.ID] {
			accepted = append(accepted, finding)
		}
	}
	payload, _ := json.MarshalIndent(map[string]any{"run_id": runID, "packet_hash": packet.PacketHash, "accepted_findings": accepted, "items": packet.Items}, "", "  ")
	return untrustedNotice + "\n\nReturn the edit proposal JSON contract. Offsets are UTF-8 byte offsets in the exact item content. Default to local scope.\n" + string(payload)
}

func validateEditProposal(packet documents.Packet, final domain.Final, proposal domain.EditProposal, confirmDocument bool) ([]plannedEdit, error) {
	if proposal.SchemaVersion != 1 || proposal.RunID != final.RunID || proposal.PacketHash != packet.PacketHash || len(proposal.Hunks) == 0 {
		return nil, fmt.Errorf("edit proposal identity or schema mismatch")
	}
	accepted := map[string]domain.Finding{}
	for _, decision := range final.Decisions {
		if decision.Outcome != "accepted" {
			continue
		}
		for _, finding := range final.Findings {
			if finding.ID == decision.FindingID {
				accepted[finding.ID] = finding
			}
		}
	}
	items := map[string]documents.Item{}
	for _, item := range packet.Items {
		items[item.ID] = item
	}
	grouped := map[string][]domain.EditHunk{}
	for _, hunk := range proposal.Hunks {
		item, ok := items[hunk.PacketItem]
		if !ok || !item.Editable {
			return nil, fmt.Errorf("hunk references missing or review-only item %q", hunk.PacketItem)
		}
		if len(hunk.FindingIDs) == 0 {
			return nil, fmt.Errorf("every hunk requires accepted finding ids")
		}
		for _, id := range hunk.FindingIDs {
			finding, ok := accepted[id]
			if !ok || finding.Anchor.PacketItem != hunk.PacketItem {
				return nil, fmt.Errorf("hunk is not tied to accepted finding %q", id)
			}
		}
		grouped[hunk.PacketItem] = append(grouped[hunk.PacketItem], hunk)
	}
	var plans []plannedEdit
	for itemID, hunks := range grouped {
		item := items[itemID]
		canonical, err := filepath.EvalSymlinks(item.SourcePath)
		if err != nil || canonical != item.SourcePath {
			return nil, fmt.Errorf("source path changed since packet creation: %s", item.SourcePath)
		}
		info, err := os.Lstat(item.SourcePath)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("source is no longer a regular file: %s", item.SourcePath)
		}
		live, err := os.ReadFile(item.SourcePath)
		if err != nil || hashText(string(live)) != item.SourceSHA256 {
			return nil, fmt.Errorf("source hash is stale for %s", item.SourcePath)
		}
		sort.Slice(hunks, func(i, j int) bool { return hunks[i].Start < hunks[j].Start })
		lastEnd := 0
		for _, hunk := range hunks {
			if hunk.SourceSHA256 != item.SourceSHA256 || hunk.Start < lastEnd || hunk.Start > hunk.End || hunk.End > len(live) || !utf8.ValidString(hunk.Replacement) || !utf8Boundary(live, hunk.Start) || !utf8Boundary(live, hunk.End) {
				return nil, fmt.Errorf("invalid, overlapping, or stale hunk for %s", item.SourcePath)
			}
			if err := validateScope(string(live), hunk, accepted, confirmDocument); err != nil {
				return nil, err
			}
			lastEnd = hunk.End
		}
		after := append([]byte(nil), live...)
		for i := len(hunks) - 1; i >= 0; i-- {
			hunk := hunks[i]
			after = append(append(append([]byte(nil), after[:hunk.Start]...), []byte(hunk.Replacement)...), after[hunk.End:]...)
		}
		plans = append(plans, plannedEdit{item: item, live: live, mode: info.Mode(), hunks: hunks, after: after})
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].item.SourcePath < plans[j].item.SourcePath })
	return plans, nil
}

func validateScope(content string, hunk domain.EditHunk, accepted map[string]domain.Finding, confirmDocument bool) error {
	switch hunk.Scope {
	case domain.EditLocal:
		allowed := false
		for _, id := range hunk.FindingIDs {
			anchor := accepted[id].Anchor
			start, end := anchor.CharOffset-256, anchor.EndOffset+256
			if start < 0 {
				start = 0
			}
			if end > len(content) {
				end = len(content)
			}
			allowed = allowed || (hunk.Start >= start && hunk.End <= end)
		}
		if !allowed {
			return fmt.Errorf("local edit exceeds accepted finding region")
		}
	case domain.EditSection:
		allowed := false
		for _, id := range hunk.FindingIDs {
			start, end := markdownSection(content, accepted[id].Anchor.CharOffset)
			allowed = allowed || (hunk.Start >= start && hunk.End <= end)
		}
		if !allowed {
			return fmt.Errorf("section edit exceeds accepted finding section")
		}
	case domain.EditDocument:
		if !confirmDocument {
			return fmt.Errorf("document-scope edit requires explicit confirmation")
		}
	default:
		return fmt.Errorf("unknown edit scope %q", hunk.Scope)
	}
	return nil
}

func markdownSection(content string, anchor int) (int, int) {
	start := strings.LastIndex(content[:min(anchor, len(content))], "\n#")
	if start >= 0 {
		start++
	} else {
		start = 0
	}
	rel := strings.Index(content[min(anchor, len(content)):], "\n#")
	if rel < 0 {
		return start, len(content)
	}
	return start, min(anchor, len(content)) + rel
}

func applyPlans(runDir, runID, packetHash string, plans []plannedEdit, now time.Time) (EditRecord, error) {
	record := EditRecord{SchemaVersion: 1, RunID: runID, PacketHash: packetHash, AppliedAt: now}
	for _, plan := range plans {
		backup := filepath.Join(runDir, "backups", hashText(plan.item.SourcePath)[:12]+".original")
		if _, err := os.Stat(backup); err == nil {
			return EditRecord{}, fmt.Errorf("backup already exists for %s", plan.item.SourcePath)
		} else if !os.IsNotExist(err) {
			return EditRecord{}, err
		}
		if err := storage.WriteFileMode(backup, plan.live, plan.mode); err != nil {
			return EditRecord{}, err
		}
		record.Files = append(record.Files, EditFileRecord{PacketItem: plan.item.ID, SourcePath: plan.item.SourcePath, BackupPath: backup, BeforeSHA256: hashText(string(plan.live)), AfterSHA256: hashText(string(plan.after))})
	}
	var applied []plannedEdit
	for _, plan := range plans {
		canonical, err := filepath.EvalSymlinks(plan.item.SourcePath)
		if err != nil || canonical != plan.item.SourcePath {
			rollbackPlans(applied)
			return EditRecord{}, fmt.Errorf("source path changed before atomic apply")
		}
		current, err := os.ReadFile(plan.item.SourcePath)
		if err != nil || hashText(string(current)) != hashText(string(plan.live)) {
			rollbackPlans(applied)
			return EditRecord{}, fmt.Errorf("source content changed before atomic apply")
		}
		if err := storage.WriteFileMode(plan.item.SourcePath, plan.after, plan.mode); err != nil {
			rollbackPlans(applied)
			return EditRecord{}, err
		}
		applied = append(applied, plan)
	}
	if err := storage.WriteJSON(filepath.Join(runDir, "edit-record.json"), record); err != nil {
		rollbackPlans(applied)
		return EditRecord{}, err
	}
	return record, nil
}

func rollbackPlans(plans []plannedEdit) {
	for _, plan := range plans {
		_ = storage.WriteFileMode(plan.item.SourcePath, plan.live, plan.mode)
	}
}

func utf8Boundary(value []byte, index int) bool {
	return index == 0 || index == len(value) || value[index]&0xc0 != 0x80
}
