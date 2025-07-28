package brain

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/dappnode/validator-tracker/internal/application/ports"
)

// This adapter is required to be used due to the web3signer blocklisting any host requesting its API that is not whitelisted.
// See https://github.com/dappnode/DAppNodePackage-web3signer-generic/blob/e50e36e6fe213f274cceefc2a089552fa6042be4/services/web3signer/entrypoint.sh#L41C28-L41C42

type BrainAdapter struct {
	BaseURL string
	client  *http.Client
}

type brainValidatorsResponse map[string][]string

func NewBrainAdapter(baseURL string) ports.BrainAdapter {
	return &BrainAdapter{
		BaseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// GetValidatorPubkeys queries /api/v0/brain/validators?format=pubkey and merges all arrays in the response
func (b *BrainAdapter) GetValidatorPubkeys() ([]string, error) {
	endpoint := fmt.Sprintf("%s/api/v0/brain/validators", b.BaseURL)

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid brain endpoint: %w", err)
	}
	q := u.Query()
	q.Set("format", "pubkey")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating brain request: %w", err)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending brain request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected brain status %d: %s", resp.StatusCode, string(body))
	}

	var result brainValidatorsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding brain response: %w", err)
	}

	var pubkeys []string
	for _, arr := range result {
		pubkeys = append(pubkeys, arr...)
	}
	return pubkeys, nil
}
