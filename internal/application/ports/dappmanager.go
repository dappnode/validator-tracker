package ports

import (
	"context"
)

type DappManagerPort interface {
	GetNotificationsEnabled(ctx context.Context) (map[string]bool, error)
}
