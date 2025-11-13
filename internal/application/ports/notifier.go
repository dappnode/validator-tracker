package ports

import "github.com/dappnode/validator-tracker/internal/application/domain"

type NotifierPort interface {
	SendValidatorLivenessNot(validators []domain.ValidatorIndex, epoch domain.Epoch, live bool) error
	SendValidatorsSlashedNot(validators []domain.ValidatorIndex, epoch domain.Epoch) error
	SendBlockProposalNot(validators []domain.ValidatorIndex, epoch domain.Epoch, proposed bool) error
	SendCommitteeNotification(validators []domain.ValidatorIndex, epoch domain.Epoch) error
}
