package tasks

import (
	"context"

	"github.com/The-True-Hooha/stellance-backend/internal/invoice"
	"github.com/The-True-Hooha/stellance-backend/internal/recurring"
	"github.com/hibiken/asynq"
)

const (
	TypeUpdateOverdueInvoices    = "invoice:update_overdue"
	TypeGenerateRecurringInvoices = "invoice:generate_recurring"
)

type UpdateOverdueInvoicesTask struct{}

func NewUpdateOverdueInvoicesTask() (*asynq.Task, error) {
	return asynq.NewTask(TypeUpdateOverdueInvoices, nil), nil
}

func HandleUpdateOverdueInvoices(ctx context.Context, t *asynq.Task) error {
	return invoice.NewInvoiceService().UpdateOverdueInvoices(ctx)
}

func NewGenerateRecurringInvoicesTask() (*asynq.Task, error) {
	return asynq.NewTask(TypeGenerateRecurringInvoices, nil), nil
}

func HandleGenerateRecurringInvoices(ctx context.Context, t *asynq.Task) error {
	return recurring.NewRecurringService().GenerateDue(ctx)
}
