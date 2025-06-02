package main

import (
	"context"
	"fmt"

	eth2client "github.com/attestantio/go-eth2-client"
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

	currentFinality, error := client.(eth2client.FinalityProvider).Finality(ctx, &api.FinalityOpts{
		State: "head",
	})
	if error != nil {
		fmt.Printf("Error retrieving finality: %v\n", error)
	}
	finalizedCheckpoint := currentFinality.Data.Finalized.Epoch // uint64

	//Check attestation duties for my validator and epoch = finalizedCheckpoint
	myValidatorIndex := uint64(1) // Replace with your validator index
	attestationDuties, err := client.(eth2client.AttesterDutiesProvider).AttesterDuties(ctx, &api.AttesterDutiesOpts{
		Epoch:   finalizedCheckpoint,
		Indices: []phase0.ValidatorIndex{phase0.ValidatorIndex(myValidatorIndex)},
	})
	if err != nil {
		fmt.Printf("Error retrieving attester duties: %v\n", err)
		return
	}

	attestationSlot := attestationDuties.Data[0].Slot                     // phase0.Slot
	attestationCommitteeIndex := attestationDuties.Data[0].CommitteeIndex // phase0.CommitteeIndex

	fmt.Printf("Attestation Slot: %d, Committee Index: %d\n", attestationSlot, attestationCommitteeIndex)

}
