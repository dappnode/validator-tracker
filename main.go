package main

import (
	"context"
	"fmt"

	"github.com/attestantio/go-eth2-client/api"
	"github.com/attestantio/go-eth2-client/http"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/rs/zerolog"
)

func main() {
	// Provide a cancellable context to the creation function.
	ctx, _ := context.WithCancel(context.Background())
	client, err := http.New(ctx,
		// WithAddress supplies the address of the beacon node, as a URL.
		http.WithAddress("http://beacon-chain.lighthouse-hoodi.dappnode:3500/"),
		// LogLevel supplies the level of logging to carry out.
		http.WithLogLevel(zerolog.WarnLevel),
	)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Connected to %s\n", client.Name())

	consensusClient := client.(*http.Service)

	// Retrieve the current finalized checkpoint. It contains latest finalized epoch.
	currentFinality, error := consensusClient.Finality(ctx, &api.FinalityOpts{
		State: "head",
	})
	if error != nil {
		fmt.Printf("Error retrieving finality: %v\n", error)
	}
	finalizedCheckpoint := currentFinality.Data.Finalized.Epoch // uint64
	fmt.Println("Current finalized Epoch:", finalizedCheckpoint)

	//Check attestation duties for my validator and epoch = finalizedCheckpoint
	myValidatorIndex := uint64(12345) // Replace with your validator index
	attestationDuties, err := consensusClient.AttesterDuties(ctx, &api.AttesterDutiesOpts{
		Epoch:   finalizedCheckpoint,
		Indices: []phase0.ValidatorIndex{phase0.ValidatorIndex(myValidatorIndex)},
	})
	if err != nil {
		fmt.Printf("Error retrieving attester duties: %v\n", err)
		return
	}

	fmt.Println("Attestation Duties complete:", attestationDuties)
	attestationSlot := attestationDuties.Data[0].Slot                     // phase0.Slot
	attestationCommitteeIndex := attestationDuties.Data[0].CommitteeIndex // phase0.CommitteeIndex
	attestationCommitteePosition := attestationDuties.Data[0].ValidatorCommitteeIndex
	fmt.Printf("Attestation Slot: %d, Committee Index: %d, Validator Committee Index: %d\n",
		attestationSlot, attestationCommitteeIndex, attestationCommitteePosition)

	included := false
	// Search up to 3 slots after the duty slot.
	for i := 0; i <= 32; i++ {
		blockSlot := attestationSlot + phase0.Slot(i)
		slotStr := fmt.Sprintf("%d", blockSlot)
		fmt.Printf("🔍 Requesting block at slot: %s\n", slotStr)

		block, err := consensusClient.SignedBeaconBlock(ctx, &api.SignedBeaconBlockOpts{
			Block: slotStr,
		})
		if err != nil {
			fmt.Printf("❌ Error for slot %d: %v\n", blockSlot, err)
			continue
		}
		if block == nil {
			fmt.Printf("❌ No block returned for slot %d\n", blockSlot)
			continue
		}
		fmt.Printf("✅ Got block with actual slot: %d\n", block.Data.Electra.Message.Slot)
		attestations := block.Data.Electra.Message.Body.Attestations
		for _, att := range attestations {
			fmt.Printf("Block slot %d: Attestation data slot: %d, index: %d\n", blockSlot, att.Data.Slot, att.Data.Index)
			if att.Data.Slot == attestationSlot && att.Data.Index == attestationCommitteeIndex {
				fmt.Println("🔍 Found matching attestation for slot and committee.")
				aggregationBits := att.AggregationBits
				byteIndex := attestationCommitteePosition / 8
				bitIndex := attestationCommitteePosition % 8
				if int(byteIndex) < len(aggregationBits) {
					bit := (aggregationBits[int(byteIndex)] >> bitIndex) & 1
					if bit == 1 {
						included = true
						fmt.Printf("✅ Validator's attestation was included in block at slot %d!\n", attestationSlot)
						break
					}
				}
			}
		}
		if included {
			break
		}
	}

	if included {
		fmt.Println("✅ Validator attested successfully!")
	} else {
		fmt.Println("❌ Validator missed attestation duty in this finalized epoch.")
	}

}
