package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dappnode/validator-tracker/internal/adapters/beacon"
	"github.com/dappnode/validator-tracker/internal/adapters/brain"
	"github.com/dappnode/validator-tracker/internal/adapters/dappmanager"
	"github.com/dappnode/validator-tracker/internal/adapters/notifier"
	"github.com/dappnode/validator-tracker/internal/application/domain"
	"github.com/dappnode/validator-tracker/internal/application/services"
	"github.com/dappnode/validator-tracker/internal/config"
	"github.com/dappnode/validator-tracker/internal/logger"
)

func main() {
	// Load config
	cfg := config.LoadConfig()
	// Print the full config in pretty JSON format
	{
		b, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			logger.Info("Loaded config: %+v", cfg)
		} else {
			logger.Info("Loaded config:\n%s", string(b))
		}
	}

	// Initialize adapters
	dappmanager := dappmanager.NewDappManagerAdapter(cfg.DappmanagerUrl, cfg.SignerDnpName)
	notifier := notifier.NewNotifier(
		cfg.NotifierUrl,
		cfg.BeaconchaUrl,
		cfg.BrainUrl,
		cfg.Network,
		cfg.SignerDnpName,
	)
	brain := brain.NewBrainAdapter(cfg.BrainUrl)
	beacon, err := beacon.NewBeaconAdapter(cfg.BeaconEndpoint)
	// TODO: do not err on initialization, allow connection errors later. See https://github.com/attestantio/go-eth2-client/issues/254
	if err != nil {
		logger.Fatal("Failed to initialize beacon adapter. A live connection is required on startup: %v", err)
	}

	// Prepare context and WaitGroup for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

	// Start the duties checker service in a goroutine
	dutiesChecker := &services.DutiesChecker{
		Beacon:            beacon,
		Brain:             brain,
		Notifier:          notifier,
		Dappmanager:       dappmanager,
		PollInterval:      1 * time.Minute,
		SlashedNotified:   make(map[domain.ValidatorIndex]bool),
		PreviouslyAllLive: true, // assume all validators were live at start
		PreviouslyOffline: false,
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
