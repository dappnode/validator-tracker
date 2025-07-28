package web3signer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dappnode/validator-tracker/internal/application/ports"
)

// Web3SignerAdapter implements ports.Web3SignerAdapter
type Web3SignerAdapter struct {
	Endpoint string
}

// KeystoreResponse models the expected JSON from /eth/v1/keystores
type KeystoreResponse struct {
	Data []struct {
		ValidatingPubkey string `json:"validating_pubkey"`
	} `json:"data"`
}

func NewWeb3SignerAdapter(endpoint string) ports.Web3SignerAdapter {
	return &Web3SignerAdapter{Endpoint: endpoint}
}

func (w *Web3SignerAdapter) GetValidatorPubkeys() ([]string, error) {
	url := fmt.Sprintf("%s/eth/v1/keystores", w.Endpoint)
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Web3Signer request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending Web3Signer request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected Web3Signer status %d: %s", resp.StatusCode, string(body))
	}

	var keystoreResp KeystoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&keystoreResp); err != nil {
		return nil, fmt.Errorf("error decoding Web3Signer response: %w", err)
	}

	pubkeys := make([]string, 0, len(keystoreResp.Data))
	for _, item := range keystoreResp.Data {
		pubkeys = append(pubkeys, item.ValidatingPubkey)
	}
	return pubkeys, nil
}
