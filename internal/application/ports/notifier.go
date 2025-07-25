package ports

import "github.com/dappnode/validator-tracker/internal/application/domain"

type NotifierPort interface {
	SendValidatorsOffNot(validators []domain.ValidatorIndex) error
	SendValidatorsOnNot(validators []domain.ValidatorIndex) error
	SendValidatorsSlashedNot(validators []domain.ValidatorIndex) error
	SendBlockProposedNot(validators []domain.ValidatorIndex, epoch int) error
}
