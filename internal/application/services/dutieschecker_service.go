package services

import (
	"context"
	"time"

	"github.com/dappnode/validator-tracker/internal/application/domain"
	"github.com/dappnode/validator-tracker/internal/application/ports"
	"github.com/dappnode/validator-tracker/internal/logger"
)

type DutiesChecker struct {
	BeaconAdapter     ports.BeaconChainAdapter
	Web3SignerAdapter ports.Web3SignerAdapter
	PollInterval      time.Duration

	lastFinalizedEpoch domain.Epoch
	CheckedEpochs      map[domain.ValidatorIndex]domain.Epoch // latest epoch checked for each validator index
}

// If at interval, ticker ticks but check has not ended, we wont start a new check, we will just wait for the next tick.
func (a *DutiesChecker) Run(ctx context.Context) {
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

func (a *DutiesChecker) checkLatestFinalizedEpoch(ctx context.Context) {
	finalizedEpoch, err := a.BeaconAdapter.GetFinalizedEpoch(ctx)
	if err != nil {
		logger.Error("Error fetching finalized epoch: %v", err)
		return
	}
	if finalizedEpoch == a.lastFinalizedEpoch {
		logger.Debug("Finalized epoch %d unchanged, skipping check.", finalizedEpoch)
		return
	}
	a.lastFinalizedEpoch = finalizedEpoch
	logger.Info("New finalized epoch %d detected.", finalizedEpoch)

	pubkeys, err := a.Web3SignerAdapter.GetValidatorPubkeys()
	if err != nil {
		logger.Error("Error fetching pubkeys from web3signer: %v", err)
		return
	}

	indices, err := a.BeaconAdapter.GetValidatorIndicesByPubkeys(ctx, pubkeys)
	if err != nil {
		logger.Error("Error fetching validator indices from beacon node: %v", err)
		return
	}
	logger.Info("Found %d validator indices active", len(indices))

	validatorIndices := a.getValidatorsToCheck(indices, finalizedEpoch)
	if len(validatorIndices) == 0 {
		logger.Debug("No validators left to check for epoch %d", finalizedEpoch)
		return
	}

	// Split proposal vs attestation logic
	a.checkProposals(ctx, finalizedEpoch, validatorIndices)
	a.checkAttestations(ctx, finalizedEpoch, validatorIndices)
}

func (a *DutiesChecker) checkProposals(
	ctx context.Context,
	finalizedEpoch domain.Epoch,
	indices []domain.ValidatorIndex,
) {
	proposerDuties, err := a.BeaconAdapter.GetProposerDuties(ctx, finalizedEpoch, indices)
	if err != nil {
		logger.Error("Error fetching proposer duties: %v", err)
		return
	}

	if len(proposerDuties) == 0 {
		logger.Warn("No proposer duties found for finalized epoch %d.", finalizedEpoch)
		return
	}

	for _, duty := range proposerDuties {
		didPropose, err := a.BeaconAdapter.DidProposeBlock(ctx, duty.Slot)
		if err != nil {
			logger.Warn("⚠️ Could not determine if block was proposed at slot %d: %v", duty.Slot, err)
			continue
		}
		if didPropose {
			logger.Info("✅ Validator %d successfully proposed a block at slot %d", duty.ValidatorIndex, duty.Slot)
		} else {
			logger.Warn("❌ Validator %d was scheduled to propose at slot %d but did not", duty.ValidatorIndex, duty.Slot)
		}
	}
}

func (a *DutiesChecker) checkAttestations(
	ctx context.Context,
	finalizedEpoch domain.Epoch,
	validatorIndices []domain.ValidatorIndex,
) {
	duties, err := a.BeaconAdapter.GetValidatorDutiesBatch(ctx, finalizedEpoch, validatorIndices)
	if err != nil {
		logger.Error("Error fetching validator duties: %v", err)
		return
	}
	if len(duties) == 0 {
		logger.Warn("No duties found for finalized epoch %d. This should not happen!", finalizedEpoch)
		return
	}

	minSlot, maxSlot := getSlotRangeForDuties(duties)
	slotAttestations := preloadSlotAttestations(ctx, a.BeaconAdapter, minSlot, maxSlot)
	committeeSizeCache := make(map[domain.Slot]domain.CommitteeSizeMap)

	for _, duty := range duties {
		attestationFound := a.checkDutyAttestation(ctx, duty, slotAttestations, committeeSizeCache)
		if !attestationFound {
			logger.Warn(" ❌ No attestation found for validator %d in finalized epoch %d",
				duty.ValidatorIndex, finalizedEpoch)
		}
		a.markCheckedThisEpoch(duty.ValidatorIndex, finalizedEpoch)
	}
}

func (a *DutiesChecker) getValidatorsToCheck(indices []domain.ValidatorIndex, epoch domain.Epoch) []domain.ValidatorIndex {
	var result []domain.ValidatorIndex
	for _, index := range indices {
		if a.wasCheckedThisEpoch(index, epoch) {
			continue
		}
		result = append(result, index)
	}
	return result
}

func (a *DutiesChecker) wasCheckedThisEpoch(index domain.ValidatorIndex, epoch domain.Epoch) bool {
	return a.CheckedEpochs[index] == epoch
}

func (a *DutiesChecker) markCheckedThisEpoch(index domain.ValidatorIndex, epoch domain.Epoch) {
	if a.CheckedEpochs == nil {
		a.CheckedEpochs = make(map[domain.ValidatorIndex]domain.Epoch)
	}
	a.CheckedEpochs[index] = epoch
}

// Important: This function assumes duties is not empty (at least one duty exists).
func getSlotRangeForDuties(duties []domain.ValidatorDuty) (domain.Slot, domain.Slot) {
	minSlot, maxSlot := duties[0].Slot, duties[0].Slot
	for _, d := range duties {
		if d.Slot < minSlot {
			minSlot = d.Slot
		}
		if d.Slot > maxSlot {
			maxSlot = d.Slot
		}
	}
	return minSlot, maxSlot
}

func preloadSlotAttestations(ctx context.Context, beacon ports.BeaconChainAdapter, minSlot, maxSlot domain.Slot) map[domain.Slot][]domain.Attestation {
	result := make(map[domain.Slot][]domain.Attestation)
	for slot := minSlot + 1; slot <= maxSlot+32; slot++ {
		att, err := beacon.GetBlockAttestations(ctx, slot)
		if err != nil {
			logger.Warn("Error fetching attestations for slot %d: %v. Was this slot missed?", slot, err)
			continue
		}
		result[slot] = att
	}
	return result
}

// checkDutyAttestation checks if there is an attestation for the given duty in the next 32 slots.
// It uses the committee size cache to avoid fetching committee sizes for every duty in repeated slots.
func (a *DutiesChecker) checkDutyAttestation(
	ctx context.Context,
	duty domain.ValidatorDuty,
	slotAttestations map[domain.Slot][]domain.Attestation,
	committeeSizeCache map[domain.Slot]domain.CommitteeSizeMap,
) bool {
	committeeSizeMap, ok := committeeSizeCache[duty.Slot]
	if !ok {
		var err error
		committeeSizeMap, err = a.BeaconAdapter.GetCommitteeSizeMap(ctx, duty.Slot)
		if err != nil {
			logger.Warn("Error fetching committee sizes for slot %d: %v", duty.Slot, err)
			return false
		}
		committeeSizeCache[duty.Slot] = committeeSizeMap
	}

	for slot := duty.Slot + 1; slot <= duty.Slot+32; slot++ {
		attestations := slotAttestations[slot]
		for _, att := range attestations {
			if att.DataSlot != duty.Slot {
				continue
			}
			if !isBitSet(att.CommitteeBits, int(duty.CommitteeIndex)) {
				continue
			}
			bitPosition := computeBitPosition(
				duty.CommitteeIndex,
				duty.ValidatorCommitteeIdx,
				att.CommitteeBits,
				committeeSizeMap,
			)
			if !isBitSet(att.AggregationBits, bitPosition) {
				continue
			}
			logger.Info("✅ Validator %d attested in committee %d for duty slot %d (included in block slot %d)",
				duty.ValidatorIndex, duty.CommitteeIndex, duty.Slot, slot)
			return true
		}
	}
	return false
}

// computeBitPosition calculates the bit position for the validator in the committee bits.
// It sums the sizes of all committees before the one the validator is in, and adds the validator's index in that committee.
// This is used to determine if the validator's aggregation bit is set in the attestation.
func computeBitPosition(
	validatorCommitteeIndex domain.CommitteeIndex,
	validatorIndexInCommittee uint64,
	committeeBits []byte,
	committeeSizeMap domain.CommitteeSizeMap,
) int {
	bitPosition := 0
	for i := 0; i < 64; i++ {
		if !isBitSet(committeeBits, i) { // if the committee bit is not set, dont add its size to final bit position
			continue
		}
		if i == int(validatorCommitteeIndex) { // We got to the committee of the validator, we can stop here.
			break
		}
		bitPosition += committeeSizeMap[domain.CommitteeIndex(i)] // Add the size of the committee to the bit position. Bit was set and it's not the committee of the validator.
	}
	bitPosition += int(validatorIndexInCommittee)
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
