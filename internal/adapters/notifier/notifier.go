package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dappnode/validator-tracker/internal/application/domain"
)

// TODO: discuss isBanner

type Notifier struct {
	BaseURL       string
	BeaconchaUrl  string
	BrainUrl      string
	Network       string
	Category      Category
	SignerDnpName string
	HTTPClient    *http.Client
}

func NewNotifier(baseURL, beaconchaUrl, brainUrl, network, signerDnpName string) *Notifier {
	category := Category(strings.ToLower(network))
	if network == "mainnet" {
		category = Ethereum
	}
	return &Notifier{
		BaseURL:       baseURL,
		BeaconchaUrl:  beaconchaUrl,
		BrainUrl:      brainUrl,
		Network:       network,
		Category:      category,
		SignerDnpName: signerDnpName,
		HTTPClient:    &http.Client{Timeout: 3 * time.Second},
	}
}

type CallToAction struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type Category string

const (
	Ethereum Category = "ethereum"
	Hoodi    Category = "hoodi"
	Holesky  Category = "holesky"
	Gnosis   Category = "gnosis"
	Lukso    Category = "lukso"
)

type Priority string

const (
	Low      Priority = "low"
	Medium   Priority = "medium"
	High     Priority = "high"
	Critical Priority = "critical"
	Info     Priority = "info"
)

type Status string

const (
	Triggered Status = "triggered"
	Resolved  Status = "resolved"
)

type NotificationPayload struct {
	Title         string        `json:"title"`
	Body          string        `json:"body"`
	Category      *Category     `json:"category,omitempty"`
	Status        *Status       `json:"status,omitempty"`
	IsBanner      *bool         `json:"isBanner,omitempty"`
	Priority      *Priority     `json:"priority,omitempty"`
	CorrelationId *string       `json:"correlationId,omitempty"`
	DnpName       *string       `json:"dnpName,omitempty"`
	CallToAction  *CallToAction `json:"callToAction,omitempty"`
}

func (n *Notifier) sendNotification(payload NotificationPayload) error {
	url := fmt.Sprintf("%s/api/v1/notifications", n.BaseURL)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notification failed with status: %s", resp.Status)
	}
	return nil
}

// SendValidatorLivenessNot sends a notification when one or more validators go offline or online.
func (n *Notifier) SendValidatorLivenessNot(validators []domain.ValidatorIndex, live bool) error {
	var title, body string
	var priority Priority
	var status Status
	var isBanner bool
	correlationId := string(domain.ValidatorLiveness)
	var callToAction *CallToAction
	beaconchaUrl := n.buildBeaconchaURL(validators)
	if beaconchaUrl != "" {
		callToAction = &CallToAction{
			Title: "View on Beaconcha",
			URL:   beaconchaUrl,
		}
	}
	if live {
		title = fmt.Sprintf("Validator(s) Online: %s", indexesToString(validators))
		body = fmt.Sprintf("✅ Validator(s) %s are back online on %s.", indexesToString(validators), n.Network)
		priority = Info
		status = Resolved
		isBanner = false
	} else {
		title = fmt.Sprintf("Validator(s) Offline: %s", indexesToString(validators))
		body = fmt.Sprintf("❌ Validator(s) %s are offline on %s.", indexesToString(validators), n.Network)
		priority = High
		status = Triggered
		isBanner = true
	}
	payload := NotificationPayload{
		Title:         title,
		Body:          body,
		Category:      &n.Category,
		Priority:      &priority,
		DnpName:       &n.SignerDnpName,
		Status:        &status,
		CorrelationId: &correlationId,
		IsBanner:      &isBanner,
		CallToAction:  callToAction,
	}
	return n.sendNotification(payload)
}

// SendValidatorsSlashedNot sends a notification when one or more validators are slashed.
func (n *Notifier) SendValidatorsSlashedNot(validators []domain.ValidatorIndex) error {
	title := fmt.Sprintf("Validator(s) Slashed: %s", indexesToString(validators))
	body := fmt.Sprintf("🚨 Validator(s) %s have been slashed on %s! Immediate attention required.", indexesToString(validators), n.Network)
	priority := Critical
	status := Triggered
	isBanner := true
	correlationId := string(domain.ValidatorSlashed)
	callToAction := &CallToAction{
		Title: "Remove validators",
		URL:   n.BrainUrl,
	}

	payload := NotificationPayload{
		Title:         title,
		Body:          body,
		Category:      &n.Category,
		Priority:      &priority,
		IsBanner:      &isBanner,
		DnpName:       &n.SignerDnpName,
		Status:        &status,
		CorrelationId: &correlationId,
		CallToAction:  callToAction,
	}
	return n.sendNotification(payload)
}

// SendBlockProposalNot sends a notification when a block is proposed or missed by one or more validators.
func (n *Notifier) SendBlockProposalNot(validators []domain.ValidatorIndex, epoch int, proposed bool) error {
	var title, body string
	var priority Priority
	var status Status = Triggered
	isBanner := true
	correlationId := string(domain.BlockProposal)
	beaconchaUrl := n.buildBeaconchaURL(validators)
	var callToAction *CallToAction
	if beaconchaUrl != "" {
		callToAction = &CallToAction{
			Title: "View on Beaconcha",
			URL:   beaconchaUrl,
		}
	}
	if proposed {
		title = fmt.Sprintf("Block Proposed: %s", indexesToString(validators))
		body = fmt.Sprintf("✅ Validator(s) %s proposed a block at epoch %d on %s.", indexesToString(validators), epoch, n.Network)
		priority = Info
	} else {
		title = fmt.Sprintf("Block Missed: %s", indexesToString(validators))
		body = fmt.Sprintf("❌ Validator(s) %s missed a block proposal at epoch %d on %s.", indexesToString(validators), epoch, n.Network)
		priority = High
	}
	payload := NotificationPayload{
		Title:         title,
		Body:          body,
		Category:      &n.Category,
		Priority:      &priority,
		IsBanner:      &isBanner,
		DnpName:       &n.SignerDnpName,
		Status:        &status,
		CorrelationId: &correlationId,
		CallToAction:  callToAction,
	}
	return n.sendNotification(payload)
}

// Helper to join validator indexes as comma-separated string
func indexesToString(indexes []domain.ValidatorIndex) string {
	var s []string
	for _, idx := range indexes {
		s = append(s, fmt.Sprintf("%d", idx))
	}
	return strings.Join(s, ",")
}

// Helper to build beaconcha URL for multiple validators
func (n *Notifier) buildBeaconchaURL(indexes []domain.ValidatorIndex) string {
	if len(indexes) == 0 || n.BeaconchaUrl == "" {
		return ""
	}
	// If only one validator, link directly to it
	if len(indexes) == 1 {
		return fmt.Sprintf("%s/validator/%d", n.BeaconchaUrl, indexes[0])
	}
	// Otherwise, link to the validators search page with comma-separated indexes
	return fmt.Sprintf("%s/validators?search=%s", n.BeaconchaUrl, indexesToString(indexes))
}
