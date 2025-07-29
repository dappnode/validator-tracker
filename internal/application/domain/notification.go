package domain

type ValidatorNotificationsEnabled map[ValidatorNotification]bool

type ValidatorNotification string

type validatorNotifications struct {
	Liveness ValidatorNotification
	Slashed  ValidatorNotification
	Proposal ValidatorNotification
}

var Notifications validatorNotifications

func InitNotifications(network string) {
	Notifications = validatorNotifications{
		Liveness: ValidatorNotification(network + "-validator-liveness"),
		Slashed:  ValidatorNotification(network + "-validator-slashed"),
		Proposal: ValidatorNotification(network + "-block-proposal"),
	}
}
