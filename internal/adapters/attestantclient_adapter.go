// internal/adapters/beaconchain_adapter.go
package adapters

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	nethttp "net/http"
	"time"

	apiv1 "github.com/attestantio/go-eth2-client/api/v1"
	"github.com/dappnode/validator-tracker/internal/application/domain"
	"github.com/dappnode/validator-tracker/internal/application/ports"
	"github.com/rs/zerolog"

	"github.com/attestantio/go-eth2-client/api"
	"github.com/attestantio/go-eth2-client/http"
	"github.com/attestantio/go-eth2-client/spec/phase0"
)

type beaconAttestantClient struct {
	client *http.Service
}

func NewBeaconAttestantAdapter(endpoint string) (ports.BeaconChainAdapter, error) {
	zerolog.SetGlobalLevel(zerolog.WarnLevel)

	customHttpClient := &nethttp.Client{
		Timeout: 2000 * time.Second,
	}

	client, err := http.New(context.Background(),
		http.WithAddress(endpoint),
		http.WithHTTPClient(customHttpClient),
		http.WithTimeout(20*time.Second), // important as attestant API overrides my timeout TODO: investigate how
	)
	if err != nil {
		return nil, err
	}

	return &beaconAttestantClient{client: client.(*http.Service)}, nil
}

// GetFinalizedEpoch retrieves the latest finalized epoch from the beacon chain.
func (b *beaconAttestantClient) GetFinalizedEpoch(ctx context.Context) (domain.Epoch, error) {
	finality, err := b.client.Finality(ctx, &api.FinalityOpts{State: "head"})
	if err != nil {
		return 0, err
	}
	return domain.Epoch(finality.Data.Finalized.Epoch), nil
}

// internal/adapters/beaconchain_adapter.go
func (b *beaconAttestantClient) GetValidatorDutiesBatch(ctx context.Context, epoch domain.Epoch, validatorIndices []domain.ValidatorIndex) ([]domain.ValidatorDuty, error) {
	// Convert to phase0.ValidatorIndex
	var indices []phase0.ValidatorIndex
	for _, idx := range validatorIndices {
		indices = append(indices, phase0.ValidatorIndex(idx))
	}

	duties, err := b.client.AttesterDuties(ctx, &api.AttesterDutiesOpts{
		Epoch:   phase0.Epoch(epoch),
		Indices: indices,
	})
	if err != nil {
		return nil, err
	}

	// Map the response to domain.ValidatorDuty
	var domainDuties []domain.ValidatorDuty
	for _, d := range duties.Data {
		domainDuties = append(domainDuties, domain.ValidatorDuty{
			Slot:                  domain.Slot(d.Slot),
			CommitteeIndex:        domain.CommitteeIndex(d.CommitteeIndex),
			ValidatorCommitteeIdx: d.ValidatorCommitteeIndex,
			ValidatorIndex:        domain.ValidatorIndex(d.ValidatorIndex), // new field
		})
	}

	return domainDuties, nil
}

func (b *beaconAttestantClient) GetValidatorDuties(ctx context.Context, epoch domain.Epoch, validatorIndex domain.ValidatorIndex) (domain.ValidatorDuty, error) {
	duties, err := b.client.AttesterDuties(ctx, &api.AttesterDutiesOpts{
		Epoch:   phase0.Epoch(epoch),
		Indices: []phase0.ValidatorIndex{phase0.ValidatorIndex(validatorIndex)},
	})
	if err != nil {
		return domain.ValidatorDuty{}, err
	}

	// 🚨 TODO: how to log this here? needed for validators loaded into web3signer but exited (no duties)
	if len(duties.Data) == 0 {
		return domain.ValidatorDuty{}, fmt.Errorf("no duties found for validator %d at epoch %d", validatorIndex, epoch)
	}

	duty := duties.Data[0]
	return domain.ValidatorDuty{
		Slot:                  domain.Slot(duty.Slot),
		CommitteeIndex:        domain.CommitteeIndex(duty.CommitteeIndex),
		ValidatorCommitteeIdx: duty.ValidatorCommitteeIndex,
	}, nil
}

// GetCommitteeSizeMap retrieves the size of each attestation committee for a specific slot.
// This is very expensive and take a long time to execute, so it should be used sparingly.
// TODO: can we get rid of this?
func (b *beaconAttestantClient) GetCommitteeSizeMap(ctx context.Context, slot domain.Slot) (domain.CommitteeSizeMap, error) {
	committees, err := b.client.BeaconCommittees(ctx, &api.BeaconCommitteesOpts{
		State: fmt.Sprintf("%d", slot),
	})
	if err != nil {
		return nil, err
	}
	sizeMap := make(domain.CommitteeSizeMap)
	for _, committee := range committees.Data {
		if domain.Slot(committee.Slot) != slot {
			continue
		}
		sizeMap[domain.CommitteeIndex(committee.Index)] = len(committee.Validators)
	}
	return sizeMap, nil
}

// GetBlockAttestations retrieves all attestations include in a slot
func (b *beaconAttestantClient) GetBlockAttestations(ctx context.Context, slot domain.Slot) ([]domain.Attestation, error) {
	block, err := b.client.SignedBeaconBlock(ctx, &api.SignedBeaconBlockOpts{
		Block: fmt.Sprintf("%d", slot),
	})
	if err != nil {
		return nil, err
	}

	var attestations []domain.Attestation
	for _, att := range block.Data.Electra.Message.Body.Attestations {
		attestations = append(attestations, domain.Attestation{
			DataSlot:        domain.Slot(att.Data.Slot),
			CommitteeBits:   att.CommitteeBits,
			AggregationBits: att.AggregationBits,
		})
	}
	return attestations, nil
}

func (b *beaconAttestantClient) GetValidatorIndicesByPubkeys(ctx context.Context, pubkeys []string) ([]domain.ValidatorIndex, error) {
	var beaconPubkeys []phase0.BLSPubKey

	// Convert hex pubkeys to BLS pubkeys
	for _, hexPubkey := range pubkeys {
		// Remove "0x" prefix if present
		if len(hexPubkey) >= 2 && hexPubkey[:2] == "0x" {
			hexPubkey = hexPubkey[2:]
		}
		bytes, err := hex.DecodeString(hexPubkey)
		if err != nil {
			return nil, errors.New("failed to decode pubkey: " + hexPubkey)
		}
		if len(bytes) != 48 {
			return nil, errors.New("invalid pubkey length for: " + hexPubkey)
		}
		var blsPubkey phase0.BLSPubKey
		copy(blsPubkey[:], bytes)
		beaconPubkeys = append(beaconPubkeys, blsPubkey)
	}

	// Only get validators in active states
	// TODO: why do I need apiv1 for this struct? is there something newer?
	validators, err := b.client.Validators(ctx, &api.ValidatorsOpts{
		State:   "head",
		PubKeys: beaconPubkeys,
		ValidatorStates: []apiv1.ValidatorState{
			apiv1.ValidatorStateActiveOngoing,
			apiv1.ValidatorStateActiveExiting,
			apiv1.ValidatorStateActiveSlashed,
		},
	})
	if err != nil {
		return nil, err
	}

	var indices []domain.ValidatorIndex
	for _, v := range validators.Data {
		indices = append(indices, domain.ValidatorIndex(v.Index))
	}
	return indices, nil
}

// GetProposerDuties retrieves proposer duties for the given epoch and validator indices.
func (b *beaconAttestantClient) GetProposerDuties(ctx context.Context, epoch domain.Epoch, indices []domain.ValidatorIndex) ([]domain.ProposerDuty, error) {
	var beaconIndices []phase0.ValidatorIndex
	for _, idx := range indices {
		beaconIndices = append(beaconIndices, phase0.ValidatorIndex(idx))
	}

	resp, err := b.client.ProposerDuties(ctx, &api.ProposerDutiesOpts{
		Epoch:   phase0.Epoch(epoch),
		Indices: beaconIndices,
	})
	if err != nil {
		return nil, err
	}

	var duties []domain.ProposerDuty
	for _, d := range resp.Data {
		duties = append(duties, domain.ProposerDuty{
			Slot:           domain.Slot(d.Slot),
			ValidatorIndex: domain.ValidatorIndex(d.ValidatorIndex),
		})
	}
	return duties, nil
}

// DidProposeBlock checks a given slot includes a block proposed
func (b *beaconAttestantClient) DidProposeBlock(ctx context.Context, slot domain.Slot) (bool, error) {
	block, err := b.client.SignedBeaconBlock(ctx, &api.SignedBeaconBlockOpts{
		Block: fmt.Sprintf("%d", slot),
	})
	if err != nil {
		// TODO: are we sure we can assume that a 404 means the block was not proposed?
		// What error code is returned in all consensus if the block is not in their state?
		if apiErr, ok := err.(*api.Error); ok && apiErr.StatusCode == 404 {
			return false, nil // Block was not proposed
		}
		return false, err // Real error
	}
	return block != nil && block.Data != nil, nil
}
