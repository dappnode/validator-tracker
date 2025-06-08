package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dappnode/validator-tracker/internal/adapters"
	"github.com/dappnode/validator-tracker/internal/application/domain"
	"github.com/dappnode/validator-tracker/internal/application/services"
	"github.com/dappnode/validator-tracker/internal/config"
	"github.com/dappnode/validator-tracker/internal/logger"
)

func main() {
	// Load config
	cfg := config.LoadConfig()
	logger.Info("Loaded config: network=%s, beaconEndpoint=%s, web3SignerEndpoint=%s",
		cfg.Network, cfg.BeaconEndpoint, cfg.Web3SignerEndpoint)

	// Fetch validator pubkeys
	web3Signer := adapters.NewWeb3SignerAdapter(cfg.Web3SignerEndpoint)
	pubkeys, err := web3Signer.GetValidatorPubkeys()
	if err != nil {
		logger.Fatal("Failed to get validator pubkeys from web3signer: %v", err)
	}
	logger.Info("Fetched %d pubkeys from web3signer", len(pubkeys))

	// Initialize beacon chain adapter
	adapter, err := adapters.NewBeaconAttestantAdapter(cfg.BeaconEndpoint)
	if err != nil {
		logger.Fatal("Failed to initialize beacon adapter: %v", err)
	}

	// Get validator indices from pubkeys
	indices, err := adapter.GetValidatorIndicesByPubkeys(context.Background(), pubkeys)
	if err != nil {
		logger.Fatal("Failed to get validator indices: %v", err)
	}
	logger.Info("Found %d validator indices active", len(indices))

	// Prepare context and WaitGroup for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

	// Start the attestation checker service in a goroutine
	logger.Info("Starting attestation checker for %d validators", len(indices))
	checker := &services.AttestationChecker{
		BeaconAdapter:     adapter,
		ValidatorIndices:  indices,
		PollInterval:      1 * time.Minute,
		CheckedValidators: make(map[domain.ValidatorIndex]domain.Epoch),
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		checker.Run(ctx)
	}()

	// Handle graceful shutdown
	handleShutdown(cancel)

	// Wait for all services to stop
	wg.Wait()
	logger.Info("All services stopped. Shutting down.")
}

// handleShutdown listens for SIGINT/SIGTERM and cancels the context
func handleShutdown(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Info("Received signal: %s. Initiating shutdown...", sig)
		cancel()
	}()
}
