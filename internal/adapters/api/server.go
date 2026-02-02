package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/dappnode/validator-tracker/internal/application/domain"
	"github.com/dappnode/validator-tracker/internal/logger"
)

// Server is a simple HTTP server for the internal API
type Server struct {
	server         *http.Server
	validatorCache *domain.ValidatorCache
}

// NewServer creates a new HTTP server
func NewServer(addr string, validatorCache *domain.ValidatorCache) *Server {
	s := &Server{
		validatorCache: validatorCache,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/validators", s.handleGetValidators)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return s
}

// Start begins listening for HTTP requests (blocking)
func (s *Server) Start() error {
	logger.Info("Starting HTTP API server on %s", s.server.Addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully stops the server
func (s *Server) Shutdown(ctx context.Context) error {
	logger.Info("Shutting down HTTP API server")
	return s.server.Shutdown(ctx)
}

// ValidatorsResponse is the JSON response for GET /api/v1/validators
type ValidatorsResponse struct {
	Validators []domain.ValidatorInfo `json:"validators"`
	Epoch      domain.Epoch           `json:"epoch"`
}

func (s *Server) handleGetValidators(w http.ResponseWriter, r *http.Request) {
	validators, epoch := s.validatorCache.Get()

	response := ValidatorsResponse{
		Validators: validators,
		Epoch:      epoch,
	}

	// Handle empty cache (service hasn't run yet)
	if response.Validators == nil {
		response.Validators = []domain.ValidatorInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Error("Failed to encode validators response: %v", err)
	}
}
