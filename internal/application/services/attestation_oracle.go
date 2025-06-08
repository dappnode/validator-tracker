package services

import (
	"context"
	"time"

	"github.com/dappnode/validator-tracker/internal/application/domain"
	"github.com/dappnode/validator-tracker/internal/application/ports"
	"github.com/dappnode/validator-tracker/internal/logger"
)

type AttestationChecker struct {
	BeaconAdapter    ports.BeaconChainAdapter
	ValidatorIndices []domain.ValidatorIndex
	PollInterval     time.Duration

	lastFinalizedEpoch domain.Epoch
	CheckedValidators  map[domain.ValidatorIndex]domain.Epoch
}

func (a *AttestationChecker) Run(ctx context.Context) {
	ticker := time.NewTicker(a.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.checkLatestFinalizedEpoch(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// checkLatestFinalizedEpoch checks the latest finalized epoch and verifies if validators have attested.
func (a *AttestationChecker) checkLatestFinalizedEpoch(ctx context.Context) {
	finalizedEpoch, err := a.BeaconAdapter.GetFinalizedEpoch(ctx)
	if err != nil {
		logger.Error("Error fetching finalized epoch: %v", err)
		return
	}

	// If finalized epoch is same as last seen, skip execution
	if finalizedEpoch == a.lastFinalizedEpoch {
		logger.Debug("Finalized epoch %d unchanged, skipping check.", finalizedEpoch)
		return
	}

	logger.Info("New finalized epoch %d detected. Checking attestations for validators...", finalizedEpoch)
	a.lastFinalizedEpoch = finalizedEpoch

	// Determine which validators still need checking for this epoch
	var validatorsToCheck []domain.ValidatorIndex
	for _, index := range a.ValidatorIndices {
		// Skip if already confirmed
		if epoch, ok := a.CheckedValidators[index]; ok && epoch == finalizedEpoch {
			logger.Debug("Duties of validator %d already successfully checked for epoch %d. Skipping.", index, finalizedEpoch)
			continue
		}
		validatorsToCheck = append(validatorsToCheck, index)
	}

	// If no validators to check, skip
	if len(validatorsToCheck) == 0 {
		logger.Info("All validators already checked for epoch %d. Skipping.", finalizedEpoch)
		return
	}

	// Fetch all duties in batch
	duties, err := a.BeaconAdapter.GetValidatorDutiesBatch(ctx, finalizedEpoch, validatorsToCheck)
	if err != nil {
		logger.Error("Error fetching validator duties: %v", err)
		return
	}

	// For each duty, check attestation inclusion
	for _, duty := range duties {
		attestationFound := false

		logger.Debug("Checking duty for validator %d in committee %d of slot %d",
			duty.ValidatorIndex, duty.CommitteeIndex, duty.Slot)

		// Check up to 32 slots after the duty slot
		for slot := duty.Slot + 1; slot <= duty.Slot+32; slot++ {
			attestations, err := a.BeaconAdapter.GetBlockAttestations(ctx, slot)
			if err != nil {
				logger.Warn("Error fetching attestations for slot %d: %v", slot, err)
				continue
			}
			// Fetch committee sizes for this block slot
			committeeSizeMap, err := a.BeaconAdapter.GetCommitteeSizeMap(ctx, duty.Slot)
			if err != nil {
				logger.Warn("Error fetching committee sizes for slot %d: %v", slot, err)
				continue
			}

			for _, att := range attestations {
				if att.DataSlot != duty.Slot {
					continue
				}
				if !isBitSet(att.CommitteeBits, int(duty.CommitteeIndex)) {
					continue
				}

				// 🟩 Compute bit position dynamically based on committeeBits
				bitPosition := computeBitPosition(
					duty.CommitteeIndex,
					duty.ValidatorCommitteeIdx,
					att.CommitteeBits,
					committeeSizeMap,
				)

				if !isBitSet(att.AggregationBits, bitPosition) {
					logger.Debug(" ❌ Validator %d not included in attestation for committee %d at slot %d (bit position %d).",
						duty.ValidatorIndex, duty.CommitteeIndex, slot, bitPosition)
					continue
				}

				// ✅ Attestation found!
				logger.Info("✅ Validator %d attested in committee %d for duty slot %d (included in block slot %d)",
					duty.ValidatorIndex, duty.CommitteeIndex, duty.Slot, slot)
				attestationFound = true
				break // Found; no need to check more attestations in this block
			}
			if attestationFound {
				break // Move on to next validator
			}
		}

		if !attestationFound {
			logger.Warn(" ❌ No attestation found for validator %d in finalized epoch %d",
				duty.ValidatorIndex, finalizedEpoch)
		}

		// Mark validator as checked for this epoch
		a.CheckedValidators[duty.ValidatorIndex] = finalizedEpoch
	}
}

// Compute the bit position of the validator in the aggregation_bits
func computeBitPosition(
	validatorCommitteeIndex domain.CommitteeIndex,
	validatorIndexInCommittee uint64,
	committeeBits []byte,
	committeeSizeMap domain.CommitteeSizeMap,
) int {
	bitPosition := 0
	for i := 0; i < 64; i++ {
		if !isBitSet(committeeBits, i) {
			continue
		}
		if i == int(validatorCommitteeIndex) {
			break
		}
		bitPosition += committeeSizeMap[domain.CommitteeIndex(i)]
	}
	bitPosition += int(validatorIndexInCommittee)
	return bitPosition
}

// isBitSet checks if a bit at a particular index is set in a bitfield
func isBitSet(bits []byte, index int) bool {
	byteIndex := index / 8
	bitIndex := index % 8

	if byteIndex >= len(bits) {
		return false
	}

	return (bits[byteIndex] & (1 << uint(bitIndex))) != 0
}
