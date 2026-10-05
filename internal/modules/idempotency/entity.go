package idempotency

import (
	"time"
)

type Status string

const (
	StatusProcessing Status = "PROCESSING"
	StatusCompleted  Status = "COMPLETED"
)

type Record struct {
	ID string

	Operation   string
	Key         string
	RequestHash string

	Status Status

	ResponseStatus *int
	ResponseBody   []byte

	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt time.Time
}
