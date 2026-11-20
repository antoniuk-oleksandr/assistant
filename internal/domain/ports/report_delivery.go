package ports

import (
	"context"
	"errors"

	"github.com/alex/assistent/internal/domain/report"
)

var ErrDeliveryNotFound = errors.New("report delivery not found")

type DeliveryTarget struct {
	Channel     string
	Destination string
}

type ReportDeliveryRepository interface {
	Create(context.Context, *report.Delivery) error
	Update(context.Context, *report.Delivery) error
	GetByResult(context.Context, int64) ([]report.Delivery, error)
	GetByResultTarget(context.Context, int64, string, string) (report.Delivery, error)
}

type ResultDeliveryProvider interface {
	Name() string
	Deliver(context.Context, report.Result, string) error
}
