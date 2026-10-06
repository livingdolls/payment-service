package application

import "errors"

var (
	ErrChannelCodeRequired          = errors.New("channel code is required")
	ErrPaymentAttemptNotProcessable = errors.New("payment attempt cannot be processed")
)
