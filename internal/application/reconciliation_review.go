package application

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/livingdolls/payment-service/internal/modules/reconciliation"
)

type ReconciliationReviewUseCase struct {
	repository reconciliation.Repository
}

func NewReconciliationReviewUseCase(repository reconciliation.Repository) *ReconciliationReviewUseCase {
	return &ReconciliationReviewUseCase{
		repository: repository,
	}
}

func (u *ReconciliationReviewUseCase) List(ctx context.Context, limit int) ([]reconciliation.ReviewItem, error) {
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	return u.repository.List(ctx, limit)
}

func (u *ReconciliationReviewUseCase) Requeue(ctx context.Context, attemptID string, actor string, reason string) (*reconciliation.ReviewAction, error) {
	if _, err := uuid.Parse(attemptID); err != nil {
		return nil, reconciliation.ErrInvalidAttemptID
	}

	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)

	if actor == "" || utf8.RuneCountInString(actor) > 100 {
		return nil, reconciliation.ErrInvalidActor
	}

	reasonLength := utf8.RuneCountInString(reason)

	if reasonLength < 10 || reasonLength > 500 {
		return nil, reconciliation.ErrInvalidReason
	}

	return u.repository.Requeue(ctx, attemptID, actor, reason)
}
