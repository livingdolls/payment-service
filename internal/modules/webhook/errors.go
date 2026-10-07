package webhook

import "errors"

var (
	ErrInvalidToken     = errors.New("invalid webhook token")
	ErrInvalidPayload   = errors.New("invalid webhook payload")
	ErrUnsupportedEvent = errors.New("unsupported webhook event")
)
