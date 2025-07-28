package domain

type ValidatorNotificationsEnabled map[ValidatorNotification]bool

// create a enum with the validator notifications
type ValidatorNotification string

const (
	ValidatorLiveness ValidatorNotification = "validator-liveness" // online/offline
	ValidatorSlashed  ValidatorNotification = "validator-slashed"
	BlockProposal     ValidatorNotification = "block-proposal"
)
