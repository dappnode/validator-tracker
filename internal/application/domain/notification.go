package domain

// create a enum with the validator notifications
type ValidatorNotification string

const (
	ValidatorLiveness ValidatorNotification = "validator-liveness" // online/offline
	ValidatorSlashed  ValidatorNotification = "validator-slashed"
	BlockProposed     ValidatorNotification = "block-proposed"
)
