package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
)

type deliveryRepository struct {
	items     []report.Delivery
	err       error
	updateErr error
}

func (r *deliveryRepository) Create(_ context.Context, item *report.Delivery) error {
	item.ID = int64(len(r.items) + 1)
	r.items = append(r.items, *item)
	return nil
}
func (r *deliveryRepository) Update(_ context.Context, item *report.Delivery) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	for i := range r.items {
		if r.items[i].ID == item.ID {
			r.items[i] = *item
		}
	}
	return nil
}
func (r *deliveryRepository) GetByResult(_ context.Context, resultID int64) ([]report.Delivery, error) {
	items := make([]report.Delivery, 0)
	for _, item := range r.items {
		if item.ResultID == resultID {
			items = append(items, item)
		}
	}
	return items, nil
}
func (r *deliveryRepository) GetByResultTarget(_ context.Context, resultID int64, channel, destination string) (report.Delivery, error) {
	if r.err != nil {
		return report.Delivery{}, r.err
	}
	for _, item := range r.items {
		if item.ResultID == resultID && item.Channel == channel && item.Destination == destination {
			return item, nil
		}
	}
	return report.Delivery{}, ports.ErrDeliveryNotFound
}

type deliveryProvider struct {
	name  string
	err   error
	calls int
}

func (p *deliveryProvider) Name() string { return p.name }
func (p *deliveryProvider) Deliver(context.Context, report.Result, string) error {
	p.calls++
	return p.err
}

func TestDeliveryServicePersistsSuccessAndPreventsDuplicateDelivery(t *testing.T) {
	repository := &deliveryRepository{}
	provider := &deliveryProvider{name: "email"}
	service := application.NewDeliveryService(repository, []ports.ResultDeliveryProvider{provider})
	result := report.Result{ID: 7, CreatedAt: time.Now()}
	target := ports.DeliveryTarget{Channel: "email", Destination: "teacher@example.com"}
	items, err := service.Deliver(context.Background(), result, []ports.DeliveryTarget{target})
	if err != nil || len(items) != 1 || items[0].Status != report.DeliverySent || provider.calls != 1 {
		t.Fatalf("items=%#v calls=%d err=%v", items, provider.calls, err)
	}
	items, err = service.Deliver(context.Background(), result, []ports.DeliveryTarget{target})
	if err != nil || len(items) != 1 || provider.calls != 1 {
		t.Fatalf("duplicate items=%#v calls=%d err=%v", items, provider.calls, err)
	}
}

func TestDeliveryServiceRecordsFailuresAndRejectsUnknownProviders(t *testing.T) {
	repository := &deliveryRepository{}
	provider := &deliveryProvider{name: "telegram", err: errors.New("provider unavailable")}
	service := application.NewDeliveryService(repository, []ports.ResultDeliveryProvider{provider})
	items, err := service.Deliver(context.Background(), report.Result{ID: 8}, []ports.DeliveryTarget{{Channel: "telegram", Destination: "chat-1"}})
	if err != nil || len(items) != 1 || items[0].Status != report.DeliveryFailed || items[0].Error != "provider unavailable" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	_, err = service.Deliver(context.Background(), report.Result{ID: 8}, []ports.DeliveryTarget{{Channel: "sms", Destination: "x"}})
	if err == nil {
		t.Fatal("unknown provider unexpectedly succeeded")
	}
	repository.err = errors.New("lookup failed")
	_, err = service.Deliver(context.Background(), report.Result{ID: 9}, []ports.DeliveryTarget{{Channel: "telegram", Destination: "chat-2"}})
	if err == nil || err.Error() != "lookup failed" {
		t.Fatalf("lookup error=%v", err)
	}
}

func TestDeliveryServiceReportsStatusPersistenceFailure(t *testing.T) {
	repository := &deliveryRepository{updateErr: errors.New("write failed")}
	service := application.NewDeliveryService(repository, []ports.ResultDeliveryProvider{&deliveryProvider{name: "email"}})
	_, err := service.Deliver(context.Background(), report.Result{ID: 10}, []ports.DeliveryTarget{{Channel: "email", Destination: "teacher@example.com"}})
	if err == nil || err.Error() != "persist delivery status: write failed" {
		t.Fatalf("error=%v", err)
	}
}

var _ ports.ReportDeliveryRepository = (*deliveryRepository)(nil)
var _ ports.ResultDeliveryProvider = (*deliveryProvider)(nil)
