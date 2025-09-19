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

	// Check block proposals (successful or missed)
	proposed, missed, err := a.checkProposals(ctx, justifiedEpoch, indices)
	if err != nil {
		logger.Error("Error checking block proposals: %v", err)
		return err
	}
	if len(proposed) > 0 && notificationsEnabled[domain.Notifications.Proposal] {
		if err := a.Notifier.SendBlockProposalNot(proposed, justifiedEpoch, true); err != nil {
			logger.Warn("Error sending block proposal notification: %v", err)
		}
	}
	if len(missed) > 0 && notificationsEnabled[domain.Notifications.Proposal] {
		if err := a.Notifier.SendBlockProposalNot(missed, justifiedEpoch, false); err != nil {
			logger.Warn("Error sending block proposal notification: %v", err)
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
