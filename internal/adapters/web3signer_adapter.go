// internal/adapters/web3signer_adapter.go
package adapters

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dappnode/validator-tracker/internal/application/ports"
)

// Web3SignerAdapter implements ports.Web3SignerAdapter
type Web3SignerAdapter struct {
	Endpoint string
}

type KeystoreResponse struct {
	Data []struct {
		ValidatingPubkey string `json:"validating_pubkey"`
	} `json:"data"`
}

func NewWeb3SignerAdapter(endpoint string) ports.Web3SignerAdapter {
	return &Web3SignerAdapter{Endpoint: endpoint}
}

func (w *Web3SignerAdapter) GetValidatorPubkeys() ([]string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/eth/v1/keystores", w.Endpoint))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch keystores: %w", err)
	}
	defer resp.Body.Close()

	var keystoreResp KeystoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&keystoreResp); err != nil {
		return nil, fmt.Errorf("failed to parse keystores: %w", err)
	}

	var pubkeys []string
	for _, item := range keystoreResp.Data {
		pubkeys = append(pubkeys, item.ValidatingPubkey)
	}
	return pubkeys, nil
}
