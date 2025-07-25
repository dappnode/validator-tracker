package dappmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/dappnode/validator-tracker/internal/application/domain"
)

// DappManagerAdapter is the adapter to interact with the DappManager API
type DappManagerAdapter struct {
	baseURL       string
	signerDnpName string
	client        *http.Client
}

// NewDappManagerAdapter creates a new DappManagerAdapter
func NewDappManagerAdapter(baseURL string, dnpName string) *DappManagerAdapter {
	return &DappManagerAdapter{
		baseURL:       baseURL,
		signerDnpName: dnpName,
		client:        &http.Client{},
	}
}

// Manifest represents the manifest of a package
type manifest struct {
	Notifications struct {
		CustomEndpoints []CustomEndpoint `json:"customEndpoints"`
	} `json:"notifications"`
}

type CustomEndpoint struct {
	Name          string `json:"name"`
	Enabled       bool   `json:"enabled"`
	Description   string `json:"description"`
	IsBanner      bool   `json:"isBanner"`
	CorrelationId string `json:"correlationId"`
	Metric        *struct {
		Treshold float64 `json:"treshold"`
		Min      float64 `json:"min"`
		Max      float64 `json:"max"`
		Unit     string  `json:"unit"`
	} `json:"metric,omitempty"`
}

// GetNotificationsEnabled retrieves the notifications from the DappManager API
func (d *DappManagerAdapter) GetNotificationsEnabled(ctx context.Context) (map[string]bool, error) {
	customEndpoints, err := d.getSignerManifestNotifications(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications from signer manifest: %w", err)
	}

	// Build a set of valid correlation IDs from domain.ValidatorNotification
	validCorrelationIDs := map[string]struct{}{
		string(domain.ValidatorLiveness): {},
		string(domain.ValidatorSlashed):  {},
		string(domain.BlockProposed):     {},
	}

	notifications := make(map[string]bool)
	for _, endpoint := range customEndpoints {
		if _, ok := validCorrelationIDs[endpoint.CorrelationId]; ok {
			notifications[endpoint.CorrelationId] = endpoint.Enabled
		}
	}

	return notifications, nil
}

// getSignerManifestNotifications gets the notifications from the Signer package manifest
func (d *DappManagerAdapter) getSignerManifestNotifications(ctx context.Context) ([]CustomEndpoint, error) {
	url := d.baseURL + "/package-manifest/" + d.signerDnpName

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for package %s: %w", d.signerDnpName, err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest for package %s: %w", d.signerDnpName, err)
	}
	defer resp.Body.Close()

	// This covers all 2xx status codes. If its not 2xx, we dont bother parsing the manifest and return an error
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code %d for package %s", resp.StatusCode, d.signerDnpName)
	}

	var manifest manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest for package %s: %w", d.signerDnpName, err)
	}

	return manifest.Notifications.CustomEndpoints, nil
}
