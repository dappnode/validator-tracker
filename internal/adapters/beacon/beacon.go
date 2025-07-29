package beacon

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	v1 "github.com/attestantio/go-eth2-client/api/v1"
	"github.com/dappnode/validator-tracker/internal/application/domain"
	"github.com/dappnode/validator-tracker/internal/application/ports"
	"github.com/rs/zerolog"

	"github.com/attestantio/go-eth2-client/api"
	_http "github.com/attestantio/go-eth2-client/http"
	"github.com/attestantio/go-eth2-client/spec/phase0"
)

type beaconAttestantClient struct {
	client *_http.Service
}

func NewBeaconAdapter(endpoint string) (ports.BeaconChainAdapter, error) {
	zerolog.SetGlobalLevel(zerolog.WarnLevel)

	customHttpClient := &http.Client{
		Timeout: 20 * time.Second,
	}

	client, err := _http.New(context.Background(),
		_http.WithAddress(endpoint),
		_http.WithHTTPClient(customHttpClient),
		_http.WithTimeout(20*time.Second), // important as attestant API overrides my timeout TODO: investigate how
	)
	if err != nil {
		return nil, err
	}

	return &beaconAttestantClient{client: client.(*_http.Service)}, nil
}

// GetFinalizedEpoch retrieves the latest finalized epoch from the beacon chain.
func (b *beaconAttestantClient) GetFinalizedEpoch(ctx context.Context) (domain.Epoch, error) {
	finality, err := b.client.Finality(ctx, &api.FinalityOpts{State: "head"})
	if err != nil {
		return 0, err
	}
	return domain.Epoch(finality.Data.Finalized.Epoch), nil
}

// GetJustifiedEpoch retrieves the latest finalized epoch from the beacon chain.
func (b *beaconAttestantClient) GetJustifiedEpoch(ctx context.Context) (domain.Epoch, error) {
	finality, err := b.client.Finality(ctx, &api.FinalityOpts{State: "head"})
	if err != nil {
		return 0, err
	}
	return domain.Epoch(finality.Data.Justified.Epoch), nil
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
		State:   "justified",
		PubKeys: beaconPubkeys,
		ValidatorStates: []v1.ValidatorState{
			v1.ValidatorStateActiveOngoing,
			v1.ValidatorStateActiveExiting,
			v1.ValidatorStateActiveSlashed,
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

func (b *beaconAttestantClient) GetValidatorsLiveness(ctx context.Context, epoch domain.Epoch, indices []domain.ValidatorIndex) (map[domain.ValidatorIndex]bool, error) {
	// Convert to phase0.ValidatorIndex
	var beaconIndices []phase0.ValidatorIndex
	for _, idx := range indices {
		beaconIndices = append(beaconIndices, phase0.ValidatorIndex(idx))
	}

	liveness, err := b.client.ValidatorLiveness(ctx, &api.ValidatorLivenessOpts{
		Epoch:   phase0.Epoch(epoch),
		Indices: beaconIndices,
	})
	if err != nil {
		return nil, err
	}

	livenessMap := make(map[domain.ValidatorIndex]bool)
	for _, v := range liveness.Data {
		livenessMap[domain.ValidatorIndex(v.Index)] = v.IsLive
	}
	return livenessMap, nil
}

// GetSlashedValidators retrieves the indices of slashed validators. In the justified state.
func (b *beaconAttestantClient) GetSlashedValidators(ctx context.Context, indices []domain.ValidatorIndex) ([]domain.ValidatorIndex, error) {
	slashed, err := b.client.Validators(ctx, &api.ValidatorsOpts{
		State: "justified",
		// Only get validators in slashed states
		ValidatorStates: []v1.ValidatorState{
			v1.ValidatorStateActiveSlashed,
			v1.ValidatorStateExitedSlashed,
		},
		Indices: make([]phase0.ValidatorIndex, len(indices)),
	})

	if err != nil {
		return nil, err
	}
	slashedIndices := make([]domain.ValidatorIndex, 0, len(slashed.Data))
	for _, v := range slashed.Data {
		slashedIndices = append(slashedIndices, domain.ValidatorIndex(v.Index))
	}
	return slashedIndices, nil
}

// enum for consensus client
type ConsensusClient string

const (
	Unknown    ConsensusClient = "unknown"
	Nimbus     ConsensusClient = "nimbus"
	Lighthouse ConsensusClient = "lighthouse"
	Teku       ConsensusClient = "teku"
	Prysm      ConsensusClient = "prysm"
	Lodestar   ConsensusClient = "lodestar"
)

// GetConsensusClient see https://ethereum.github.io/beacon-APIs/#/Node/getNodeVersion. Does not throw an error if the client is not available
func (b *beaconAttestantClient) GetConsensusClient(ctx context.Context) ConsensusClient {
	resp, err := b.client.NodeClient(ctx)
	if err != nil || resp == nil {
		return Unknown
	}

	switch resp.Data {
	case "nimbus":
		return Nimbus
	case "lighthouse":
		return Lighthouse
	case "teku":
		return Teku
	case "prysm":
		return Prysm
	case "lodestar":
		return Lodestar
	default:
		return Unknown
	}
}
