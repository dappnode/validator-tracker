package sqlite

import (
	"context"
	"database/sql" // basic sql
	"fmt"

	_ "github.com/mattn/go-sqlite3" // additional driver for sqlite
)

// Implements ports.ValidatorStoragePort

type SQLiteStorage struct {
	DB *sql.DB
}

func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("failed to migrate sqlite db: %w", err)
	}
	return &SQLiteStorage{DB: db}, nil
}

func migrate(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS validator_epoch_status (
			index INTEGER NOT NULL,
			epoch INTEGER NOT NULL,
			liveness BOOLEAN,
			in_sync_committee BOOLEAN,
			sync_committee_reward INTEGER,
			attestation_reward INTEGER, 
			slashed BOOLEAN,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (index, epoch)
		);`,
		`CREATE TABLE IF NOT EXISTS validator_block_proposals (
			index INTEGER NOT NULL,
			slot INTEGER NOT NULL,
			epoch INTEGER NOT NULL,
			block_reward INTEGER,
			PRIMARY KEY (index, slot)
		);`,
		`CREATE TABLE IF NOT EXISTS validators (
			index INTEGER PRIMARY KEY,
			label TEXT,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_epoch ON validator_epoch_status(epoch);`,
		`CREATE INDEX IF NOT EXISTS idx_validator_epoch ON validator_epoch_status(index, epoch);`,
		`CREATE INDEX IF NOT EXISTS idx_proposals_epoch ON validator_block_proposals(epoch);`,
		`CREATE INDEX IF NOT EXISTS idx_proposals_slot ON validator_block_proposals(slot);`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// Upsert = Insert or Update. If a record with the same primary key exists, it updates the existing record.
// If the record does not exist, it inserts a new record.

// UpsertValidatorEpochStatus inserts or updates validator epoch status. It will update fields if the record exists.
// If any of parameters are nil, the corresponding fields will be set to NULL in the database.
func (s *SQLiteStorage) UpsertValidatorEpochStatus(ctx context.Context, index uint64, epoch uint64, liveness *bool, inSyncCommittee *bool, syncCommitteeReward *uint64, attestationReward *uint64, slashed *bool) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO validator_epoch_status (index, epoch, liveness, in_sync_committee, sync_committee_reward, attestation_reward, slashed)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(index, epoch) DO UPDATE SET
			liveness=excluded.liveness,
			in_sync_committee=excluded.in_sync_committee,
			sync_committee_reward=excluded.sync_committee_reward,
			attestation_reward=excluded.attestation_reward,
			slashed=excluded.slashed,
			updated_at=CURRENT_TIMESTAMP;`,
		index, epoch, liveness, inSyncCommittee, syncCommitteeReward, attestationReward, slashed,
	)
	return err
}

// UpsertValidatorBlockProposal inserts or updates a block proposal for a validator. It will update the block_reward if the record exists.
// If blockReward is nil, the block_reward field will be set to NULL in the database.
func (s *SQLiteStorage) UpsertValidatorBlockProposal(ctx context.Context, index uint64, slot uint64, epoch uint64, blockReward *uint64) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO validator_block_proposals (index, slot, epoch, block_reward)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(index, slot) DO UPDATE SET
			block_reward=excluded.block_reward;`,
		index, slot, epoch, blockReward,
	)
	return err
}

func (s *SQLiteStorage) UpsertValidatorMetadata(ctx context.Context, index uint64, label *string) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO validators (index, label)
		VALUES (?, ?)
		ON CONFLICT(index) DO UPDATE SET
			label=excluded.label;`,
		index, label,
	)
	return err
}
