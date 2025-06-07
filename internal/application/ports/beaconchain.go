package ports

import (
	"context"

	"github.com/dappnode/validator-tracker/internal/application/domain"
)

// ports/beaconchain_adapter.go
type BeaconChainAdapter interface {
	GetFinalizedEpoch(ctx context.Context) (domain.Epoch, error)
	GetValidatorDutiesBatch(ctx context.Context, epoch domain.Epoch, validatorIndices []domain.ValidatorIndex) ([]domain.ValidatorDuty, error)
	GetCommitteeSizeMap(ctx context.Context, slot domain.Slot) (domain.CommitteeSizeMap, error)
	GetBlockAttestations(ctx context.Context, slot domain.Slot) ([]domain.Attestation, error)
	GetValidatorIndicesByPubkeys(ctx context.Context, pubkeys []string) ([]domain.ValidatorIndex, error)
}
