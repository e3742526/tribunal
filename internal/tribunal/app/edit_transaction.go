package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

type editTransactionFile struct {
	SchemaVersion int         `json:"schema_version"`
	SourcePath    string      `json:"source_path"`
	RecoveryPath  string      `json:"recovery_path"`
	BeforeSHA256  string      `json:"before_sha256"`
	AfterSHA256   string      `json:"after_sha256"`
	Mode          os.FileMode `json:"mode"`
	Applied       bool        `json:"applied"`
}

type editTransaction struct {
	SchemaVersion int                   `json:"schema_version"`
	RunID         string                `json:"run_id"`
	PacketHash    string                `json:"packet_hash"`
	Operation     string                `json:"operation"`
	Phase         string                `json:"phase"`
	Files         []editTransactionFile `json:"files"`
	UpdatedAt     time.Time             `json:"updated_at"`
	Reason        string                `json:"reason,omitempty"`
}

type transactionPlan struct {
	source, recovery string
	before, after    []byte
	mode             os.FileMode
}

func (s *Service) executeEditTransaction(runDir, runID, packetHash, operation string, plans []transactionPlan) (editTransaction, error) {
	tx := editTransaction{SchemaVersion: 1, RunID: runID, PacketHash: packetHash, Operation: operation, Phase: "prepared", UpdatedAt: s.now()}
	for _, plan := range plans {
		tx.Files = append(tx.Files, editTransactionFile{SchemaVersion: 1, SourcePath: plan.source, RecoveryPath: plan.recovery, BeforeSHA256: hashText(string(plan.before)), AfterSHA256: hashText(string(plan.after)), Mode: plan.mode})
	}
	path := filepath.Join(runDir, "edit-transaction.json")
	if err := storage.WriteJSON(path, tx); err != nil {
		return tx, err
	}
	if err := s.editFault("after-prepare"); err != nil {
		return tx, err
	}
	for i, plan := range plans {
		// A leftover backup from a rolled-back or reverted transaction must
		// not block retry forever. Identical content is safely reused;
		// different content is archived under a content-addressed name so
		// nothing an old record references is ever destroyed.
		if existing, err := os.ReadFile(plan.recovery); err == nil {
			if hashText(string(existing)) != tx.Files[i].BeforeSHA256 {
				archived := plan.recovery + ".superseded-" + hashText(string(existing))[:24]
				if err := os.Rename(plan.recovery, archived); err != nil {
					return tx, fmt.Errorf("archive stale recovery backup for %s: %w", plan.source, err)
				}
			}
		} else if !os.IsNotExist(err) {
			return tx, err
		}
		if err := storage.WriteFileMode(plan.recovery, plan.before, plan.mode); err != nil {
			return tx, err
		}
		if err := s.editFault(fmt.Sprintf("after-backup-%d", i)); err != nil {
			return tx, err
		}
	}
	tx.Phase, tx.UpdatedAt = "applying", s.now()
	if err := storage.WriteJSON(path, tx); err != nil {
		return tx, err
	}
	for i, plan := range plans {
		canonical, err := filepath.EvalSymlinks(plan.source)
		if err != nil || canonical != plan.source {
			return tx, fmt.Errorf("source path changed before atomic apply")
		}
		current, err := os.ReadFile(plan.source)
		if err != nil || hashText(string(current)) != tx.Files[i].BeforeSHA256 {
			return tx, fmt.Errorf("source content changed before atomic apply")
		}
		if err := storage.WriteFileMode(plan.source, plan.after, plan.mode); err != nil {
			return tx, err
		}
		if err := s.editFault(fmt.Sprintf("after-source-%d", i)); err != nil {
			return tx, err
		}
		tx.Files[i].Applied, tx.UpdatedAt = true, s.now()
		if err := storage.WriteJSON(path, tx); err != nil {
			return tx, err
		}
	}
	tx.Phase, tx.UpdatedAt = "applied", s.now()
	if err := storage.WriteJSON(path, tx); err != nil {
		return tx, err
	}
	return tx, nil
}

func (s *Service) editFault(point string) error {
	if s.EditFault == nil {
		return nil
	}
	if err := s.EditFault(point); err != nil {
		return fmt.Errorf("edit fault at %s: %w", point, err)
	}
	return nil
}

func loadEditTransaction(runDir string) (editTransaction, error) {
	var tx editTransaction
	if err := storage.ReadJSONStrict(filepath.Join(runDir, "edit-transaction.json"), &tx); err != nil {
		return editTransaction{}, err
	}
	if tx.SchemaVersion != 1 || tx.RunID == "" || tx.PacketHash == "" || (tx.Operation != "apply" && tx.Operation != "revert") || tx.Phase == "" || len(tx.Files) == 0 {
		return editTransaction{}, fmt.Errorf("invalid edit transaction")
	}
	for _, file := range tx.Files {
		if file.SchemaVersion != 1 || file.SourcePath == "" || file.RecoveryPath == "" || len(file.BeforeSHA256) != 64 || len(file.AfterSHA256) != 64 {
			return editTransaction{}, fmt.Errorf("invalid edit transaction file")
		}
	}
	return tx, nil
}

func validateEditRecord(record EditRecord) error {
	if record.SchemaVersion != 1 || record.RunID == "" || record.PacketHash == "" || len(record.Files) == 0 || record.AppliedAt.IsZero() {
		return fmt.Errorf("invalid edit record")
	}
	for _, file := range record.Files {
		if file.SchemaVersion != 1 || file.PacketItem == "" || file.SourcePath == "" || file.BackupPath == "" || len(file.BeforeSHA256) != 64 || len(file.AfterSHA256) != 64 {
			return fmt.Errorf("invalid edit file record")
		}
	}
	return nil
}

func (s *Service) setTransactionPhase(runDir string, tx *editTransaction, phase, reason string) error {
	tx.Phase, tx.Reason, tx.UpdatedAt = phase, reason, s.now()
	return storage.WriteJSON(filepath.Join(runDir, "edit-transaction.json"), tx)
}

func (s *Service) rollbackEditTransaction(runDir string, tx *editTransaction, reason string) error {
	for _, file := range tx.Files {
		canonical, err := filepath.EvalSymlinks(file.SourcePath)
		if err != nil || canonical != file.SourcePath {
			return s.manualHold(runDir, tx, "source path changed during rollback")
		}
		live, err := os.ReadFile(file.SourcePath)
		if err != nil {
			return s.manualHold(runDir, tx, "source unavailable during rollback")
		}
		liveHash := hashText(string(live))
		if liveHash == file.BeforeSHA256 {
			continue
		}
		if liveHash != file.AfterSHA256 {
			return s.manualHold(runDir, tx, "source has user or ambiguous changes")
		}
		before, err := os.ReadFile(file.RecoveryPath)
		if err != nil || hashText(string(before)) != file.BeforeSHA256 {
			return s.manualHold(runDir, tx, "recovery backup is unavailable or corrupt")
		}
		if err := storage.WriteFileMode(file.SourcePath, before, file.Mode); err != nil {
			return s.manualHold(runDir, tx, "rollback write failed")
		}
	}
	if err := s.reconcileRolledBackRecord(runDir, *tx); err != nil {
		return s.manualHold(runDir, tx, err.Error())
	}
	if err := s.setTransactionPhase(runDir, tx, "rolled_back", reason); err != nil {
		return err
	}
	var final domain.Final
	if err := storage.ReadJSONStrict(filepath.Join(runDir, "final.json"), &final); err == nil {
		if err := domain.ValidateFinal(final); err != nil {
			return err
		}
		if err := storage.Transition(runDir, storage.StateForFinal(final)); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Service) reconcileRolledBackRecord(runDir string, tx editTransaction) error {
	path := filepath.Join(runDir, "edit-record.json")
	var record EditRecord
	if err := storage.ReadJSONStrict(path, &record); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := validateEditRecord(record); err != nil {
		return err
	}
	if record.RunID != tx.RunID || record.PacketHash != tx.PacketHash {
		return fmt.Errorf("edit record conflicts with transaction")
	}
	if tx.Operation == "apply" {
		now := s.now()
		record.RolledBackAt = &now
	} else {
		record.RevertedAt = nil
	}
	return storage.WriteJSON(path, record)
}

func (s *Service) manualHold(runDir string, tx *editTransaction, reason string) error {
	_ = s.setTransactionPhase(runDir, tx, "manual_hold", reason)
	return fmt.Errorf("edit transaction requires manual recovery: %s", reason)
}

func verifyTransactionAfter(tx editTransaction) error {
	for _, file := range tx.Files {
		canonical, err := filepath.EvalSymlinks(file.SourcePath)
		if err != nil || canonical != file.SourcePath {
			return fmt.Errorf("transaction source path changed")
		}
		live, err := os.ReadFile(file.SourcePath)
		if err != nil || hashText(string(live)) != file.AfterSHA256 {
			return fmt.Errorf("transaction source does not match committed hash")
		}
	}
	return nil
}

func transactionFailure(primary, rollback error) error {
	if rollback == nil {
		return primary
	}
	return fmt.Errorf("%v; rollback failed: %w", primary, rollback)
}

func (s *Service) recoverEditTransaction(runDir string, final domain.Final) error {
	tx, err := loadEditTransaction(runDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	wantCommitted := (tx.Operation == "apply" && final.EditsApplied) || (tx.Operation == "revert" && !final.EditsApplied)
	switch tx.Phase {
	case "committed":
		if !wantCommitted {
			return s.manualHold(runDir, &tx, "committed transaction conflicts with terminal state")
		}
		return nil
	case "rolled_back":
		return nil
	case "manual_hold":
		return fmt.Errorf("edit transaction requires manual recovery: %s", tx.Reason)
	}
	if wantCommitted {
		if err := verifyTransactionAfter(tx); err != nil {
			return s.manualHold(runDir, &tx, err.Error())
		}
		return s.setTransactionPhase(runDir, &tx, "committed", "recovered after terminal publication")
	}
	return s.rollbackEditTransaction(runDir, &tx, "recovered before terminal publication")
}
