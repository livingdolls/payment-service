package webhook

import "context"

type Repository interface {
	Store(ctx context.Context, event *Event) (bool, error)
}
