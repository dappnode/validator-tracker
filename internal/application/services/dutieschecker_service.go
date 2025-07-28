package services

import (
	"context"
	"time"

	"github.com/dappnode/validator-tracker/internal/application/domain"
	"github.com/dappnode/validator-tracker/internal/application/ports"
	"github.com/dappnode/validator-tracker/internal/logger"
)

type DutiesChecker struct {
	Beacon      ports.BeaconChainAdapter
	Signer      ports.Web3SignerAdapter
	Notifier    ports.NotifierPort
	Dappmanager ports.DappManagerPort

	PollInterval       time.Duration
	lastJustifiedEpoch domain.Epoch
	CheckedEpochs      map[domain.ValidatorIndex]domain.Epoch // latest epoch checked for each validator index

	// lastLivenessState tracks the last liveness notification sent: nil = no notification sent yet (first run),
	// true = last notification was online, false = last notification was offline
	lastLivenessState *bool
}

// If at interval, ticker ticks but check has not ended, we wont start a new check, we will just wait for the next tick.
func (a *DutiesChecker) Run(ctx context.Context) {
	ticker := time.NewTicker(a.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			notificationsEnabled, err := a.Dappmanager.GetNotificationsEnabled(ctx)
			if err != nil {
				logger.Warn("Error fetching notifications enabled, notification will not be sent: %v", err)
			}
			a.checkLatestJustifiedEpoch(ctx, notificationsEnabled)
		case <-ctx.Done():
			return
		}
	}
}

func (a *DutiesChecker) checkLatestJustifiedEpoch(ctx context.Context, notificationsEnabled domain.ValidatorNotificationsEnabled) {
	justifiedEpoch, err := a.Beacon.GetJustifiedEpoch(ctx)
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

	pubkeys, err := a.Signer.GetValidatorPubkeys()
	if err != nil {
		logger.Error("Error fetching pubkeys from web3signer: %v", err)
		return
	}

	indices, err := a.Beacon.GetValidatorIndicesByPubkeys(ctx, pubkeys)
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

	// Liveness notification logic
	offline, _, allLive, err := a.checkLiveness(ctx, justifiedEpoch, validatorIndices)
	if err != nil {
		logger.Error("Error checking liveness for validators: %v", err)
		return
	}
	if len(offline) > 0 && (a.lastLivenessState == nil || *a.lastLivenessState) {
		if notificationsEnabled[domain.ValidatorLiveness] {
			if err := a.Notifier.SendValidatorLivenessNot(offline, false); err != nil {
				logger.Warn("Error sending validator liveness notification: %v", err)
			}
		}
		val := false
		a.lastLivenessState = &val
	}
	if allLive && (a.lastLivenessState == nil || !*a.lastLivenessState) {
		if notificationsEnabled[domain.ValidatorLiveness] {
			if err := a.Notifier.SendValidatorLivenessNot(validatorIndices, true); err != nil {
				logger.Warn("Error sending validator liveness notification: %v", err)
			}
		}
		val := true
		a.lastLivenessState = &val
	}

	// Block proposal notification logic
	proposed, missed, err := a.checkProposals(ctx, justifiedEpoch, validatorIndices)
	if err != nil {
		logger.Error("Error checking block proposals: %v", err)
		return
	}
	if len(proposed) > 0 && notificationsEnabled[domain.BlockProposal] {
		if err := a.Notifier.SendBlockProposalNot(proposed, int(justifiedEpoch), true); err != nil {
			logger.Warn("Error sending block proposal notification: %v", err)
		}
	}
	if len(missed) > 0 && notificationsEnabled[domain.BlockProposal] {
		if err := a.Notifier.SendBlockProposalNot(missed, int(justifiedEpoch), false); err != nil {
			logger.Warn("Error sending block proposal notification: %v", err)
		}
	}

	// Mark validators as checked
	for _, index := range validatorIndices {
		a.CheckedEpochs[index] = justifiedEpoch
	}
}

func (a *DutiesChecker) checkLiveness(
	ctx context.Context,
	epochToTrack domain.Epoch,
	indices []domain.ValidatorIndex,
) (offline []domain.ValidatorIndex, online []domain.ValidatorIndex, allLive bool, err error) {
	if len(indices) == 0 {
		logger.Warn("No validators to check liveness for in epoch %d", epochToTrack)
		return nil, nil, false, nil
	}

	livenessMap, err := a.Beacon.GetValidatorsLiveness(ctx, epochToTrack, indices)
	if err != nil {
		return nil, nil, false, err
	}

	allLive = true
	for _, idx := range indices {
		isLive, ok := livenessMap[idx]
		if !ok || !isLive {
			offline = append(offline, idx)
			allLive = false
		} else {
			online = append(online, idx)
		}
	}
	return offline, online, allLive, nil
}

func (a *DutiesChecker) checkProposals(
	ctx context.Context,
	epochToTrack domain.Epoch,
	indices []domain.ValidatorIndex,
) (proposed []domain.ValidatorIndex, missed []domain.ValidatorIndex, err error) {
	proposerDuties, err := a.Beacon.GetProposerDuties(ctx, epochToTrack, indices)
	if err != nil {
		return nil, nil, err
	}

	if len(proposerDuties) == 0 {
		logger.Warn("No proposer duties for any validators in epoch %d", epochToTrack)
		return nil, nil, nil
	}

	for _, duty := range proposerDuties {
		didPropose, err := a.Beacon.DidProposeBlock(ctx, duty.Slot)
		if err != nil {
			logger.Warn("⚠️ Could not determine if block was proposed at slot %d: %v", duty.Slot, err)
			continue
		}
		if didPropose {
			proposed = append(proposed, duty.ValidatorIndex)
			logger.Info("✅ Validator %d successfully proposed a block at slot %d", duty.ValidatorIndex, duty.Slot)
		} else {
			missed = append(missed, duty.ValidatorIndex)
			logger.Warn("❌ Validator %d was scheduled to propose at slot %d but did not", duty.ValidatorIndex, duty.Slot)
		}
	}
	return proposed, missed, nil
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
