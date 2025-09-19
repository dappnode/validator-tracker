package ports

import (
	"context"
)

// ValidatorStoragePort defines methods for persisting validator duty and proposal data
// in a hexagonal architecture.
type ValidatorStoragePort interface {
	// UpsertValidatorEpochStatus inserts or updates validator epoch status.
	UpsertValidatorEpochStatus(ctx context.Context, index uint64, epoch uint64, liveness *bool, inSyncCommittee *bool, syncCommitteeReward *uint64, attestationReward *uint64, slashed *bool) error

	// UpsertValidatorBlockProposal inserts or updates a block proposal for a validator.
	UpsertValidatorBlockProposal(ctx context.Context, index uint64, slot uint64, epoch uint64, blockReward *uint64) error

	// UpsertValidatorMetadata inserts or updates validator metadata.
	UpsertValidatorMetadata(ctx context.Context, index uint64, label *string) error
}
