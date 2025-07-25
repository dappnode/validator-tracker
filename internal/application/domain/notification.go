package domain

// create a enum with the validator notifications
type ValidatorNotification string

const (
	ValidatorOffline ValidatorNotification = "validator-offline"
	ValidatorOnline  ValidatorNotification = "validator-online"
	ValidatorSlashed ValidatorNotification = "validator-slashed"
	BlockProposed    ValidatorNotification = "block-proposed"
)
