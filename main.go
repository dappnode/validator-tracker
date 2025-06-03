package main

import (
	"context"
	"fmt"
	"time"

	"github.com/attestantio/go-eth2-client/api"
	"github.com/attestantio/go-eth2-client/http"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/rs/zerolog"
)

const validatorIndex = 480347 // Replace with the actual validator index you want to query

func main() {
	// Provide a cancellable context to the creation function.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := http.New(ctx,
		// WithAddress supplies the address of the beacon node, as a URL.
		http.WithAddress("http://beacon-chain.lighthouse-hoodi.dappnode:3500/"),
		// LogLevel supplies the level of logging to carry out.
		http.WithLogLevel(zerolog.WarnLevel),
	)
	if err != nil {
		panic(err)
	}

	consensusClient := client.(*http.Service)
	fmt.Printf("Connected to %s\n", client.Name())

	finalityCheckpoint, _ := consensusClient.Finality(context.Background(), &api.FinalityOpts{
		State: "head",
	})
	if finalityCheckpoint == nil {
		fmt.Println("No finality checkpoint found.")
		return
	}

	finalityEpoch := finalityCheckpoint.Data.Finalized.Epoch
	fmt.Printf("Finality checkpoint at epoch %d\n", finalityEpoch)

	validatorDuties, err := consensusClient.AttesterDuties(context.Background(),
		&api.AttesterDutiesOpts{
			Epoch:   finalityEpoch,
			Indices: []phase0.ValidatorIndex{phase0.ValidatorIndex(validatorIndex)},
		})
	if err != nil {
		fmt.Printf("Error fetching attester duties: %v\n", err)
		return
	}

	committeeIndex := validatorDuties.Data[0].CommitteeIndex
	ValidatorCommitteeIdx := validatorDuties.Data[0].ValidatorCommitteeIndex
	slotToAttest := validatorDuties.Data[0].Slot

	fmt.Printf("Validator %d is in committee %d at slot %d and validator committee index %d \n",
		validatorIndex, committeeIndex, slotToAttest, ValidatorCommitteeIdx)

	// Retrieve all beacon committees defined for the slotToAttest
	// This is necessary to know how many validators are in each committee.
	completeCommittees, err := consensusClient.BeaconCommittees(context.Background(),
		&api.BeaconCommitteesOpts{
			State: fmt.Sprintf("%d", slotToAttest),
		})
	if err != nil {
		fmt.Printf("Error fetching beacon committees: %v\n", err)
		return
	}

	// Store in a map the number of validators in each committee for the slotToAttest
	committeeSizeMap := make(map[phase0.CommitteeIndex]int)
	for _, committee := range completeCommittees.Data {
		if committee.Slot != slotToAttest {
			continue
		}
		committeeSizeMap[committee.Index] = len(committee.Validators)
	}
	// Print the committee sizes for debuggin

	// Get the attestations for the slots slotToAttest +1 to slotToAttest + 4
	// This is overkill. The attestant library doesnt have an endpoint to get only the attestations for a specific slot, we have to get the full slot block.
	// loop over slots slotToAttest+1 to slotToAttest+4 and perform the check
	for slot := slotToAttest + 1; slot <= slotToAttest+4; slot++ {
		fullBlock, err := consensusClient.SignedBeaconBlock(context.Background(),
			&api.SignedBeaconBlockOpts{
				Block: fmt.Sprintf("%d", slot),
			})
		if err != nil {
			fmt.Printf("Error fetching signed beacon block at slot %d: %v\n", slot, err)
			continue
		}

		attestations := fullBlock.Data.Electra.Message.Body.Attestations
		fmt.Printf("Checking %d attestations in block at slot %d...\n", len(attestations), slot)

		// Iterate over the attestations and check if there is an attestation that matches the following criteria:
		// - Attestation data slot is equal to slotToAttest
		// - committeeBit is 1 for the "committeeIndex"
		// - AggregationBit is 1 for the "ValidatorCommitteeIdx"
		// You will need to calculate the committeeBit suposing that there can be 64 committees.
		// You will need to take into account that the aggregationBit is a bitlist of the validators in each committee ordered. This means that if
		// the committee has 64 validators, the first 64 bits of the aggregationBit correspond to the first committee, the next 64 bits to the second committee, and so on.
		for _, attestation := range attestations {
			if attestation.Data.Slot != slotToAttest {
				continue
			}

			// Check if the committeeBit is set for the committeeIndex
			if !isBitSet(attestation.CommitteeBits, int(committeeIndex)) {
				continue
			}

			// Calculate bit position in aggregation bits
			bitPosition := 0
			for index, size := range committeeSizeMap {
				if index < committeeIndex {
					bitPosition += size
				}
			}
			bitPosition += int(ValidatorCommitteeIdx)

			if !isBitSet(attestation.AggregationBits, bitPosition) {
				continue
			}

			fmt.Printf("✅ Found attestation for validator %d in committee %d for slot %d (included in block %d)\n",
				validatorIndex, committeeIndex, slotToAttest, slot)
		}
	}
}

// isBitSet returns true if the bit at position 'index' is set (1) in the given byte slice.
func isBitSet(bits []byte, index int) bool {
	byteIndex := index / 8
	bitIndex := index % 8

	if byteIndex >= len(bits) {
		return false
	}

	return (bits[byteIndex] & (1 << uint(bitIndex))) != 0
}
