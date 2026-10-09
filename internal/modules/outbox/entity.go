package outbox

import "time"

type Status string

const (
	StatusPending    Status = "PENDING"
	StatusProcessing Status = "PROCESSING"
	StatusPublished  Status = "PUBLISHED"
	StatusDead       Status = "DEAD"
)

type Event struct {
	ID       string
	EventKey string

	AggregateType string
	AggregateID   string

	EventType string

	Payload []byte

	Status Status

	AttemptCount int

	AvailableAt time.Time

	ClaimToken *string
	ClaimedAt  *time.Time

	PublishedAt *time.Time

	LastError *string

	CreatedAt time.Time
	UpdatedAt time.Time
}
