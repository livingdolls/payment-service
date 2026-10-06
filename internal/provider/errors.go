package provider

import "fmt"

type ErrorKind string

const (
	ErrorKindRejected       ErrorKind = "REJECTED"
	ErrorKindUnknownOutcome ErrorKind = "UNKNOWN_OUTCOME"
)

type Error struct {
	Kind       ErrorKind
	StatusCode int
	Code       string
	Message    string
	Err        error
}

func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf(
			"provider error: kind=%s code=%s message=%s",
			e.Kind,
			e.Code,
			e.Message,
		)
	}

	if e.Err != nil {
		return fmt.Sprintf(
			"provider error: kind=%s: %v", e.Kind, e.Err,
		)
	}

	return fmt.Sprintf(
		"provider error: kind=%s", e.Kind,
	)
}

func (e *Error) Unwrap() error {
	return e.Err
}
