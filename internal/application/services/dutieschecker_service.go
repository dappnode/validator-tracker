package services

import (
	"context"
	"slices"
	"time"

	"github.com/dappnode/validator-tracker/internal/application/domain"
	"github.com/dappnode/validator-tracker/internal/application/ports"
	"github.com/dappnode/validator-tracker/internal/logger"
)

type DutiesChecker struct {
	Beacon      ports.BeaconChainAdapter
	Brain       ports.BrainAdapter
	Notifier    ports.NotifierPort
	Dappmanager ports.DappManagerPort

	PollInterval       time.Duration
	lastJustifiedEpoch domain.Epoch
	lastRunHadError    bool

	SlashedNotified map[domain.ValidatorIndex]bool

	// Tracking previous states for notifications
	PreviouslyAllLive bool
	PreviouslyOffline bool

	ValidatorStorage ports.ValidatorStoragePort // <-- added field for storage
}

func (a *DutiesChecker) Run(ctx context.Context) {
	ticker := time.NewTicker(a.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			justifiedEpoch, err := a.Beacon.GetJustifiedEpoch(ctx)
			if err != nil {
				logger.Error("Error fetching justified epoch: %v", err)
				a.lastRunHadError = true
				continue
			}

			if justifiedEpoch == a.lastJustifiedEpoch && !a.lastRunHadError {
				logger.Debug("Justified epoch %d unchanged and last run was successful, skipping check.", justifiedEpoch)
				continue
			}

			a.lastJustifiedEpoch = justifiedEpoch
			a.lastRunHadError = a.performChecks(ctx, justifiedEpoch) != nil

		case <-ctx.Done():
			return
		}
	}
}

func (a *DutiesChecker) performChecks(ctx context.Context, justifiedEpoch domain.Epoch) error {
	logger.Info("New justified epoch %d detected.", justifiedEpoch)

	notificationsEnabled, err := a.Dappmanager.GetNotificationsEnabled(ctx)
	if err != nil {
		logger.Warn("Error fetching notifications enabled, notification will not be sent: %v", err)
	}

	pubkeys, err := a.Brain.GetValidatorPubkeys()
	if err != nil {
		logger.Error("Error fetching pubkeys from brain: %v", err)
		return err
	}

	if len(pubkeys) == 0 {
		logger.Debug("No pubkeys found in brain for epoch %d, nothing to check.", justifiedEpoch)
		return nil
	}

	indices, err := a.Beacon.GetValidatorIndicesByPubkeys(ctx, pubkeys)
	if err != nil {
		logger.Error("Error fetching validator indices from beacon node: %v", err)
		return err
	}
	logger.Info("Found %d validator indices active", len(indices))

	if len(indices) == 0 {
		logger.Debug("No validators found to check for epoch %d", justifiedEpoch)
		return nil
	}

	offline, online, allLive, err := a.checkLiveness(ctx, justifiedEpoch, indices)
	if err != nil {
		logger.Error("Error checking liveness for validators: %v", err)
		return err
	}

	// Debug print: show offline, online, and allLive status
	logger.Debug("Liveness check: offline=%v, online=%v, allLive=%v", offline, online, allLive)
	logger.Debug("Previously all live: %v, previously offline: %v", a.PreviouslyAllLive, a.PreviouslyOffline)

	// Check for the first condition: 1 or more validators offline when all were previously live
	if len(offline) > 0 && a.PreviouslyAllLive {
		if notificationsEnabled[domain.Notifications.Liveness] {
			logger.Debug("Sending notification for validators going offline: %v", offline)
			if err := a.Notifier.SendValidatorLivenessNot(offline, justifiedEpoch, false); err != nil {
				logger.Warn("Error sending validator liveness notification: %v", err)
			}
		}
		a.PreviouslyAllLive = false
		a.PreviouslyOffline = true
	}

	// Check for the second condition: all validators online after 1 or more were offline
	if allLive && a.PreviouslyOffline {
		if notificationsEnabled[domain.Notifications.Liveness] {
			logger.Debug("Sending notification for all validators back online: %v", indices)
			if err := a.Notifier.SendValidatorLivenessNot(indices, justifiedEpoch, true); err != nil {
				logger.Warn("Error sending validator liveness notification: %v", err)
			}
		}
		a.PreviouslyAllLive = true
		a.PreviouslyOffline = false
	}

	// Fetch sync committee membership for this epoch
	syncCommitteeMap, err := a.Beacon.GetSyncCommittee(ctx, justifiedEpoch, indices)
	if err != nil {
		logger.Warn("Error fetching sync committee membership: %v", err)
	} else {
		var inCommittee []domain.ValidatorIndex
		for _, idx := range indices {
			if syncCommitteeMap[idx] {
				inCommittee = append(inCommittee, idx)
			}
		}
		if len(inCommittee) > 0 && notificationsEnabled[domain.Notifications.Committee] {
			logger.Info("Sending committee notification for validators: %v", inCommittee)
			if err := a.Notifier.SendCommitteeNotification(inCommittee, justifiedEpoch); err != nil {
				logger.Warn("Error sending committee notification: %v", err)
			}
		}
	}

	// Check for slashed validators
	slashed, err := a.Beacon.GetSlashedValidators(ctx, indices)
	if err != nil {
		logger.Error("Error fetching slashed validators: %v", err)
		return err
	}

	// Notify about slashed validators only if they haven't been notified before
	var toNotify []domain.ValidatorIndex
	for _, index := range slashed {
		if !a.SlashedNotified[index] {
			toNotify = append(toNotify, index)
			a.SlashedNotified[index] = true
		}
	}

	if len(toNotify) > 0 && notificationsEnabled[domain.Notifications.Slashed] {
		if err := a.Notifier.SendValidatorsSlashedNot(toNotify, justifiedEpoch); err != nil {
			logger.Warn("Error sending validator slashed notification: %v", err)
		}
	}

	// Check block proposals (successful or missed)
	proposed, missed, err := a.checkProposals(ctx, justifiedEpoch, indices)
	if err != nil {
		logger.Error("Error checking block proposals: %v", err)
		return err
	}
	if len(proposed) > 0 && notificationsEnabled[domain.Notifications.Proposal] {
		proposedIndices := make([]domain.ValidatorIndex, len(proposed))
		for i, p := range proposed {
			proposedIndices[i] = p.ValidatorIndex
		}
		if err := a.Notifier.SendBlockProposalNot(proposedIndices, justifiedEpoch, true); err != nil {
			logger.Warn("Error sending block proposal notification: %v", err)
		}
	}
	if len(missed) > 0 && notificationsEnabled[domain.Notifications.Proposal] {
		missedIndices := make([]domain.ValidatorIndex, len(missed))
		for i, m := range missed {
			missedIndices[i] = m.ValidatorIndex
		}
		if err := a.Notifier.SendBlockProposalNot(missedIndices, justifiedEpoch, false); err != nil {
			logger.Warn("Error sending block proposal notification: %v", err)
		}
	}

	// Persist block proposal data
	for _, p := range proposed {
		if err := a.ValidatorStorage.UpsertValidatorBlockProposal(ctx, uint64(p.ValidatorIndex), uint64(p.Slot), uint64(justifiedEpoch), nil); err != nil {
			logger.Warn("Failed to persist block proposal for validator %d: %v", p.ValidatorIndex, err)
		}
	}
	for _, m := range missed {
		if err := a.ValidatorStorage.UpsertValidatorBlockProposal(ctx, uint64(m.ValidatorIndex), uint64(m.Slot), uint64(justifiedEpoch), nil); err != nil {
			logger.Warn("Failed to persist missed proposal for validator %d: %v", m.ValidatorIndex, err)
		}
	}

	// Persist liveness, committee, attestation reward, and slashed status for all checked validators
	for _, idx := range indices {
		var liveness *bool
		isLive := slices.Contains(online, idx)
		liveness = new(bool)
		*liveness = isLive

		var inSyncCommittee *bool
		if syncCommitteeMap != nil {
			val := syncCommitteeMap[idx]
			inSyncCommittee = new(bool)
			*inSyncCommittee = val
		}

		var slashedFlag *bool
		isSlashed := slices.Contains(slashed, idx)
		slashedFlag = new(bool)
		*slashedFlag = isSlashed

		var attestationReward *uint64
		var syncCommitteeReward *uint64
		// TODO: fetch attestation and sync committee rewards if available. For now, set to nil.
		attestationReward = nil
		syncCommitteeReward = nil

		if err := a.ValidatorStorage.UpsertValidatorEpochStatus(ctx, uint64(idx), uint64(justifiedEpoch), liveness, inSyncCommittee, syncCommitteeReward, attestationReward, slashedFlag); err != nil {
			logger.Warn("Failed to persist epoch status for validator %d: %v", idx, err)
		}
	}

	return nil
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
			logger.Warn("❌ Validator %d was not seen in epoch %d", idx, epochToTrack)
		} else {
			online = append(online, idx)
			logger.Info("✅ Validator %d seen in epoch %d", idx, epochToTrack)
		}
	}
	return offline, online, allLive, nil
}

func (a *DutiesChecker) checkProposals(
	ctx context.Context,
	epochToTrack domain.Epoch,
	indices []domain.ValidatorIndex,
) (proposed []domain.ProposerDuty, missed []domain.ProposerDuty, err error) {
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
			proposed = append(proposed, duty)
			logger.Info("✅ Validator %d successfully proposed a block at slot %d", duty.ValidatorIndex, duty.Slot)
		} else {
			missed = append(missed, duty)
			logger.Warn("❌ Validator %d was scheduled to propose at slot %d but did not", duty.ValidatorIndex, duty.Slot)
		}
	}
	return proposed, missed, nil
}
