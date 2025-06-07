package services

import (
	"context"
	"time"

	"slices"

	"github.com/dappnode/validator-tracker/internal/application/domain"
	"github.com/dappnode/validator-tracker/internal/application/ports"
	"github.com/dappnode/validator-tracker/internal/logger"
)

type AttestationChecker struct {
	BeaconAdapter    ports.BeaconChainAdapter
	ValidatorIndices []domain.ValidatorIndex
	PollInterval     time.Duration

	lastFinalizedEpoch domain.Epoch // Track last seen finalized epoch
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

	// Update last seen finalized epoch
	a.lastFinalizedEpoch = finalizedEpoch
	logger.Info("New finalized epoch %d detected. Checking attestations for %d validators...", finalizedEpoch, len(a.ValidatorIndices))

	// Fetch all validator duties in a single call
	duties, err := a.BeaconAdapter.GetValidatorDutiesBatch(ctx, finalizedEpoch, a.ValidatorIndices)
	if err != nil {
		logger.Error("Error fetching validator duties: %v", err)
		return
	}

	// Results map to track if each validator attested
	attestationResults := make(map[domain.ValidatorIndex]bool)

	// TODO: careful setting duty as missed if program is interrupted here
	for _, duty := range duties {
		logger.Info("Checking duties for validator %d in committee %d for slot %d", duty.ValidatorIndex, duty.CommitteeIndex, duty.Slot)
		committeeSizeMap, err := a.BeaconAdapter.GetCommitteeSizeMap(ctx, duty.Slot)
		if err != nil {
			logger.Warn("Error fetching committee sizes for slot %d: %v", duty.Slot, err)
			continue
		}
		logger.Info("comitee gotten")
		found := false
		for slot := duty.Slot + 1; slot <= duty.Slot+32; slot++ {
			attestations, err := a.BeaconAdapter.GetBlockAttestations(ctx, slot)
			if err != nil {
				logger.Warn("Error fetching attestations for slot %d: %v", slot, err)
				continue
			}

			for _, att := range attestations {
				if att.DataSlot != duty.Slot {
					continue
				}
				if !isBitSet(att.CommitteeBits, int(duty.CommitteeIndex)) {
					continue
				}

				bitPosition := computeBitPosition(duty.CommitteeIndex, duty.ValidatorCommitteeIdx, committeeSizeMap)
				if !isBitSet(att.AggregationBits, bitPosition) {
					continue
				}

				found = true
				logger.Info("✅ Found attestation for validator %d in committee %d for slot %d (included in block %d)",
					duty.ValidatorIndex, duty.CommitteeIndex, duty.Slot, slot)
				break
			}
			if found {
				break
			}
		}
		attestationResults[duty.ValidatorIndex] = found
	}

	// Summary report
	logger.Info("Attestation summary for finalized epoch %d:", finalizedEpoch)
	for _, idx := range a.ValidatorIndices {
		if attestationResults[idx] {
			logger.Info("✅ Validator %d attested successfully", idx)
		} else {
			logger.Warn("❌ Validator %d missed attestation", idx)
		}
	}
}

func computeBitPosition(committeeIndex domain.CommitteeIndex, validatorCommitteeIdx uint64, committeeSizeMap domain.CommitteeSizeMap) int {
	indices := make([]domain.CommitteeIndex, 0, len(committeeSizeMap))
	for index := range committeeSizeMap {
		indices = append(indices, index)
	}
	slices.Sort(indices)

	bitPosition := 0
	for _, index := range indices {
		if index < committeeIndex {
			bitPosition += committeeSizeMap[index]
		}
	}
	bitPosition += int(validatorCommitteeIdx)
	return bitPosition
}

func isBitSet(bits []byte, index int) bool {
	byteIndex := index / 8
	bitIndex := index % 8

	if byteIndex >= len(bits) {
		return false
	}

	return (bits[byteIndex] & (1 << uint(bitIndex))) != 0
}
