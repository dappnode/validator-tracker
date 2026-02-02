package domain

import "sync"

// ValidatorInfo represents a validator with full beacon chain data
type ValidatorInfo struct {
	Index   ValidatorIndex `json:"index"`
	Balance uint64         `json:"balance"`
	Status  string         `json:"status"`

	Validator ValidatorData `json:"validator"`
}

// ValidatorData contains the validator's registration data
type ValidatorData struct {
	Pubkey                     string `json:"pubkey"`
	WithdrawalCredentials      string `json:"withdrawal_credentials"`
	EffectiveBalance           uint64 `json:"effective_balance"`
	Slashed                    bool   `json:"slashed"`
	ActivationEligibilityEpoch Epoch  `json:"activation_eligibility_epoch"`
	ActivationEpoch            Epoch  `json:"activation_epoch"`
	ExitEpoch                  Epoch  `json:"exit_epoch"`
	WithdrawableEpoch          Epoch  `json:"withdrawable_epoch"`
}

// ValidatorCache is a thread-safe cache for validator information
// Updated by DutiesChecker on epoch changes, read by HTTP handlers
type ValidatorCache struct {
	mu         sync.RWMutex
	validators []ValidatorInfo
	lastEpoch  Epoch
}

func NewValidatorCache() *ValidatorCache {
	return &ValidatorCache{}
}

// Update replaces the cached validator list (called by DutiesChecker on epoch change)
func (c *ValidatorCache) Update(validators []ValidatorInfo, epoch Epoch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.validators = validators
	c.lastEpoch = epoch
}

// Get returns the cached validators and the epoch they were fetched at
func (c *ValidatorCache) Get() ([]ValidatorInfo, Epoch) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// Return a copy to prevent race conditions if caller modifies the slice
	result := make([]ValidatorInfo, len(c.validators))
	copy(result, c.validators)
	return result, c.lastEpoch
}
