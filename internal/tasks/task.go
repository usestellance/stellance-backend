package tasks

import (
	"context"

	"github.com/The-True-Hooha/stellance-backend/internal/invoice"
	"github.com/hibiken/asynq"
)

const TypeUpdateOverdueInvoices = "invoice:update_overdue"

type UpdateOverdueInvoicesTask struct{}

func NewUpdateOverdueInvoicesTask() (*asynq.Task, error) {
	return asynq.NewTask(TypeUpdateOverdueInvoices, nil), nil
}

func HandleUpdateOverdueInvoices(ctx context.Context, t *asynq.Task) error {
	return invoice.NewInvoiceService().UpdateOverdueInvoices(ctx)
}
