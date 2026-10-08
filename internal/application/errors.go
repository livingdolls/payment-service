package application

import "errors"

var (
	ErrChannelCodeRequired          = errors.New("channel code is required")
	ErrPaymentAttemptNotProcessable = errors.New("payment attempt cannot be processed")
	ErrProviderStateConflict        = errors.New("provider state conflicts with current payment state")
	ErrProviderIdentityMismatch     = errors.New("provider identity mismatch")
)
