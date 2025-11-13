package ports

import (
	"context"

	"github.com/dappnode/validator-tracker/internal/application/domain"
)

// ports/beaconchain_adapter.go
type BeaconChainAdapter interface {
	GetFinalizedEpoch(ctx context.Context) (domain.Epoch, error)
	GetJustifiedEpoch(ctx context.Context) (domain.Epoch, error)
	GetValidatorDutiesBatch(ctx context.Context, epoch domain.Epoch, validatorIndices []domain.ValidatorIndex) ([]domain.ValidatorDuty, error)
	GetCommitteeSizeMap(ctx context.Context, slot domain.Slot) (domain.CommitteeSizeMap, error)
	GetBlockAttestations(ctx context.Context, slot domain.Slot) ([]domain.Attestation, error)
	GetValidatorIndicesByPubkeys(ctx context.Context, pubkeys []string) ([]domain.ValidatorIndex, error)
	GetSlashedValidators(ctx context.Context, indices []domain.ValidatorIndex) ([]domain.ValidatorIndex, error)

	GetProposerDuties(ctx context.Context, epoch domain.Epoch, indices []domain.ValidatorIndex) ([]domain.ProposerDuty, error)
	DidProposeBlock(ctx context.Context, slot domain.Slot) (bool, error)

	GetValidatorsLiveness(ctx context.Context, epoch domain.Epoch, indices []domain.ValidatorIndex) (map[domain.ValidatorIndex]bool, error)
	GetSyncCommittee(ctx context.Context, epoch domain.Epoch, indices []domain.ValidatorIndex) (map[domain.ValidatorIndex]bool, error)
}
