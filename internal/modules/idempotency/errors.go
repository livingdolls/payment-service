package idempotency

import "errors"

var (
	ErrKeyRequired = errors.New("idempotency key is required")
	ErrKeyTooLong  = errors.New("idempotency key is too long")
	ErrConflict    = errors.New("idempotency key was already used with different request")
	ErrInProgress  = errors.New("idempotency request is still processing")
)
