package payment

import (
	"context"
	"fmt"
)

type AttemptService struct {
	repository AttemptRepository
}

func NewAttemptService(repository AttemptRepository) *AttemptService {
	return &AttemptService{
		repository: repository,
	}
}

func (s *AttemptService) MarkRequestingProvider(ctx context.Context, attempt *PaymentAttempt) error {
	expectedVersion := attempt.Version

	if err := attempt.TransitionTo(AttemptStatusRequestingProvider); err != nil {
		return fmt.Errorf("transition payment attempt: %w", err)
	}

	if err := s.repository.UpdateAttempt(
		ctx,
		attempt,
		expectedVersion,
	); err != nil {
		return fmt.Errorf("update payment attempt: %w", err)
	}

	return nil
}
