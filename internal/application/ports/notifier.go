package ports

import "github.com/dappnode/validator-tracker/internal/application/domain"

type NotifierPort interface {
	SendValidatorLivenessNot(validators []domain.ValidatorIndex, live bool) error
	SendValidatorsSlashedNot(validators []domain.ValidatorIndex) error
	SendBlockProposalNot(validators []domain.ValidatorIndex, epoch int, proposed bool) error
}
