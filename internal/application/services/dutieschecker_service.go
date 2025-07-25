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

	lastJustifiedEpoch domain.Epoch
	CheckedEpochs      map[domain.ValidatorIndex]domain.Epoch // latest epoch checked for each validator index
}

// If at interval, ticker ticks but check has not ended, we wont start a new check, we will just wait for the next tick.
func (a *DutiesChecker) Run(ctx context.Context) {
	ticker := time.NewTicker(a.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.chechLatestJustifiedEpoch(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (a *DutiesChecker) chechLatestJustifiedEpoch(ctx context.Context) {
	justifiedEpoch, err := a.BeaconAdapter.GetJustifiedEpoch(ctx)
	if err != nil {
		logger.Error("Error fetching justified epoch: %v", err)
		return
	}
	if justifiedEpoch == a.lastJustifiedEpoch {
		logger.Debug("Justified epoch %d unchanged, skipping check.", justifiedEpoch)
		return
	}
	a.lastJustifiedEpoch = justifiedEpoch
	logger.Info("New justified epoch %d detected.", justifiedEpoch)

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

	validatorIndices := a.getValidatorsToCheck(indices, justifiedEpoch)
	if len(validatorIndices) == 0 {
		logger.Debug("No validators left to check for epoch %d", justifiedEpoch)
		return
	}

	// Initialize a map to track success of both checks
	proposalChecked := make(map[domain.ValidatorIndex]bool)
	livenessChecked := make(map[domain.ValidatorIndex]bool)

	a.checkProposals(ctx, justifiedEpoch, validatorIndices, proposalChecked)
	a.checkLiveness(ctx, justifiedEpoch, validatorIndices, livenessChecked)

	// Mark validators as checked only if both checks succeeded
	for _, index := range validatorIndices {
		if proposalChecked[index] && livenessChecked[index] {
			a.CheckedEpochs[index] = justifiedEpoch
		}
	}
}

func (a *DutiesChecker) checkLiveness(
	ctx context.Context,
	epochToTrack domain.Epoch,
	indices []domain.ValidatorIndex,
	livenessChecked map[domain.ValidatorIndex]bool,
) {
	if len(indices) == 0 {
		logger.Warn("No validators to check liveness for in epoch %d", epochToTrack)
		return
	}

	livenessMap, err := a.BeaconAdapter.GetValidatorsLiveness(ctx, epochToTrack, indices)
	if err != nil {
		logger.Error("Error checking liveness for validators: %v", err)
		return
	}

	for _, index := range indices {
		isLive, ok := livenessMap[index]
		if !ok {
			logger.Warn("⚠️ Liveness info not found for validator %d in epoch %d", index, epochToTrack)
			continue
		}
		livenessChecked[index] = true
		if !isLive {
			logger.Warn("❌ Validator %d is not live in epoch %d", index, epochToTrack)
		} else {
			logger.Info("✅ Validator %d is live in epoch %d", index, epochToTrack)
		}
	}
}

func (a *DutiesChecker) checkProposals(
	ctx context.Context,
	epochToTrack domain.Epoch,
	indices []domain.ValidatorIndex,
	proposalChecked map[domain.ValidatorIndex]bool,
) {
	proposerDuties, err := a.BeaconAdapter.GetProposerDuties(ctx, epochToTrack, indices)
	if err != nil {
		logger.Error("Error fetching proposer duties: %v", err)
		return
	}

	if len(proposerDuties) == 0 {
		logger.Warn("No proposer duties for any validators in epoch %d", epochToTrack)
		return
	}

	for _, duty := range proposerDuties {
		didPropose, err := a.BeaconAdapter.DidProposeBlock(ctx, duty.Slot)
		if err != nil {
			logger.Warn("⚠️ Could not determine if block was proposed at slot %d: %v", duty.Slot, err)
			continue
		}
		proposalChecked[duty.ValidatorIndex] = true
		if didPropose {
			logger.Info("✅ Validator %d successfully proposed a block at slot %d", duty.ValidatorIndex, duty.Slot)
		} else {
			logger.Warn("❌ Validator %d was scheduled to propose at slot %d but did not", duty.ValidatorIndex, duty.Slot)
		}
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
