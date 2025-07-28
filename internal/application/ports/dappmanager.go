package ports

import (
	"context"

	"github.com/dappnode/validator-tracker/internal/application/domain"
)

type DappManagerPort interface {
	GetNotificationsEnabled(ctx context.Context) (domain.ValidatorNotificationsEnabled, error)
}
