package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dappnode/validator-tracker/internal/adapters/api"
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

	// Initialize network-specific notification correlation IDs
	domain.InitNotifications(cfg.Network)

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

	// Initialize shared validator cache for HTTP API
	validatorCache := domain.NewValidatorCache()

	// Start the duties checker service in a goroutine
	dutiesChecker := &services.DutiesChecker{
		Beacon:            beacon,
		Brain:             brain,
		Notifier:          notifier,
		Dappmanager:       dappmanager,
		PollInterval:      time.Duration(cfg.PollIntervalSeconds) * time.Second,
		SlashedNotified:   make(map[domain.ValidatorIndex]bool),
		PreviouslyAllLive: true, // assume all validators were live at start
		PreviouslyOffline: false,
		ValidatorCache:    validatorCache,
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		dutiesChecker.Run(ctx)
	}()

	// Start HTTP API server
	apiServer := api.NewServer(":8080", validatorCache)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := apiServer.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP API server error: %v", err)
		}
	}()

	// Graceful shutdown for HTTP server
	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := apiServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP API server shutdown error: %v", err)
		}
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
