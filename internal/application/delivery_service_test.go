package application_test

import (
	"context"
	"errors"
	"time"

	"github.com/alex/assistent/internal/application"
	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type deliveryRepository struct {
	items     []report.Delivery
	err       error
	createErr error
	updateErr error
}

func (r *deliveryRepository) Create(_ context.Context, item *report.Delivery) error {
	if r.createErr != nil {
		return r.createErr
	}
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

var _ = Describe("delivery service", func() {
	It("persists success and prevents duplicate delivery", func() {
		repository := &deliveryRepository{}
		provider := &deliveryProvider{name: "email"}
		service := application.NewDeliveryService(repository, []ports.ResultDeliveryProvider{provider})
		result := report.Result{ID: 7, CreatedAt: time.Now()}
		target := ports.DeliveryTarget{Channel: "email", Destination: "teacher@example.com"}
		items, err := service.Deliver(context.Background(), result, []ports.DeliveryTarget{target})
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].Status).To(Equal(report.DeliverySent))
		Expect(provider.calls).To(Equal(1))
		items, err = service.Deliver(context.Background(), result, []ports.DeliveryTarget{target})
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(provider.calls).To(Equal(1))
	})

	It("records failures and rejects unknown providers", func() {
		repository := &deliveryRepository{}
		provider := &deliveryProvider{name: "telegram", err: errors.New("provider unavailable")}
		service := application.NewDeliveryService(repository, []ports.ResultDeliveryProvider{provider})
		items, err := service.Deliver(context.Background(), report.Result{ID: 8}, []ports.DeliveryTarget{{Channel: "telegram", Destination: "chat-1"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].Status).To(Equal(report.DeliveryFailed))
		Expect(items[0].Error).To(Equal("provider unavailable"))
		provider.err = nil
		items, err = service.Deliver(context.Background(), report.Result{ID: 8}, []ports.DeliveryTarget{{Channel: "telegram", Destination: "chat-1"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(items[0].Status).To(Equal(report.DeliverySent))
		Expect(provider.calls).To(Equal(2))
		_, err = service.Deliver(context.Background(), report.Result{ID: 8}, []ports.DeliveryTarget{{Channel: "sms", Destination: "x"}})
		Expect(err).To(HaveOccurred())
		repository.err = errors.New("lookup failed")
		_, err = service.Deliver(context.Background(), report.Result{ID: 9}, []ports.DeliveryTarget{{Channel: "telegram", Destination: "chat-2"}})
		Expect(err).To(MatchError("lookup failed"))
	})

	It("reports status persistence failures", func() {
		repository := &deliveryRepository{updateErr: errors.New("write failed")}
		service := application.NewDeliveryService(repository, []ports.ResultDeliveryProvider{&deliveryProvider{name: "email"}})
		_, err := service.Deliver(context.Background(), report.Result{ID: 10}, []ports.DeliveryTarget{{Channel: "email", Destination: "teacher@example.com"}})
		Expect(err).To(MatchError("persist delivery status: write failed"))
	})

	It("reports create failures", func() {
		service := application.NewDeliveryService(&deliveryRepository{createErr: errors.New("create failed")}, []ports.ResultDeliveryProvider{&deliveryProvider{name: "email"}})
		_, err := service.Deliver(context.Background(), report.Result{ID: 11}, []ports.DeliveryTarget{{Channel: "email", Destination: "teacher@example.com"}})
		Expect(err).To(MatchError("persist delivery: create failed"))
	})

	It("propagates retry attempt errors", func() {
		now := time.Now().UTC()
		repository := &deliveryRepository{items: []report.Delivery{{ID: 1, ResultID: 12, Channel: "email", Destination: "teacher@example.com", Status: report.DeliveryFailed, CreatedAt: now}}, updateErr: errors.New("retry persistence failed")}
		service := application.NewDeliveryService(repository, []ports.ResultDeliveryProvider{&deliveryProvider{name: "email", err: errors.New("retry failed")}})
		_, err := service.Deliver(context.Background(), report.Result{ID: 12}, []ports.DeliveryTarget{{Channel: "email", Destination: "teacher@example.com"}})
		Expect(err).To(MatchError("persist delivery status: retry persistence failed"))
	})
})

var _ ports.ReportDeliveryRepository = (*deliveryRepository)(nil)
var _ ports.ResultDeliveryProvider = (*deliveryProvider)(nil)
