package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dappnode/validator-tracker/internal/adapters/beacon"
	"github.com/dappnode/validator-tracker/internal/adapters/dappmanager"
	"github.com/dappnode/validator-tracker/internal/adapters/notifier"
	"github.com/dappnode/validator-tracker/internal/adapters/web3signer"
	"github.com/dappnode/validator-tracker/internal/application/domain"
	"github.com/dappnode/validator-tracker/internal/application/services"
	"github.com/dappnode/validator-tracker/internal/config"
	"github.com/dappnode/validator-tracker/internal/logger"
)

//TODO: Implement dev mode with commands example

func main() {
	// Load config
	cfg := config.LoadConfig()
	logger.Info("Loaded e config: network=%s, beaconEndpoint=%s, web3SignerEndpoint=%s",
		cfg.Network, cfg.BeaconEndpoint, cfg.Web3SignerEndpoint)

	// Initialize adapters
	dappmanager := dappmanager.NewDappManagerAdapter(cfg.DappmanagerUrl, cfg.SignerDnpName)
	notifier := notifier.NewNotifier(
		cfg.NotifierUrl,
		cfg.BeaconchaUrl,
		cfg.Network,
		cfg.SignerDnpName,
	)
	web3Signer := web3signer.NewWeb3SignerAdapter(cfg.Web3SignerEndpoint)
	beacon, err := beacon.NewBeaconAdapter(cfg.BeaconEndpoint)
	if err != nil {
		logger.Fatal("Failed to initialize beacon adapter: %v", err)
	}

	// Prepare context and WaitGroup for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

	// Start the duties checker service in a goroutine
	dutiesChecker := &services.DutiesChecker{
		Beacon:        beacon,
		Signer:        web3Signer,
		Notifier:      notifier,
		Dappmanager:   dappmanager,
		PollInterval:  1 * time.Minute,
		CheckedEpochs: make(map[domain.ValidatorIndex]domain.Epoch),
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		dutiesChecker.Run(ctx)
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
