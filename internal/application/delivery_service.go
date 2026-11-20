package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alex/assistent/internal/domain/ports"
	"github.com/alex/assistent/internal/domain/report"
)

type DeliveryService struct {
	repository ports.ReportDeliveryRepository
	providers  map[string]ports.ResultDeliveryProvider
	clock      func() time.Time
}

func NewDeliveryService(repository ports.ReportDeliveryRepository, providers []ports.ResultDeliveryProvider) *DeliveryService {
	resolved := make(map[string]ports.ResultDeliveryProvider, len(providers))
	for _, provider := range providers {
		resolved[provider.Name()] = provider
	}
	return &DeliveryService{repository: repository, providers: resolved, clock: func() time.Time { return time.Now().UTC() }}
}

// Deliver records one durable delivery state per configured target. A sent
// target is idempotent; failed targets remain visible for the retry workflow.
func (s *DeliveryService) Deliver(ctx context.Context, result report.Result, targets []ports.DeliveryTarget) ([]report.Delivery, error) {
	deliveries := make([]report.Delivery, 0, len(targets))
	for _, target := range targets {
		if _, ok := s.providers[target.Channel]; !ok {
			return deliveries, fmt.Errorf("delivery provider %q is not configured", target.Channel)
		}
		existing, err := s.repository.GetByResultTarget(ctx, result.ID, target.Channel, target.Destination)
		if err == nil {
			if existing.Status == report.DeliverySent {
				deliveries = append(deliveries, existing)
				continue
			}
			delivery := existing
			if err := s.attempt(ctx, &delivery, result, target); err != nil {
				return deliveries, err
			}
			deliveries = append(deliveries, delivery)
			continue
		}
		if err != nil && !errors.Is(err, ports.ErrDeliveryNotFound) {
			return deliveries, err
		}
		delivery := report.Delivery{ResultID: result.ID, Channel: target.Channel, Destination: target.Destination, Status: report.DeliveryPending, CreatedAt: s.clock()}
		if err := s.repository.Create(ctx, &delivery); err != nil {
			return deliveries, fmt.Errorf("persist delivery: %w", err)
		}
		if err := s.attempt(ctx, &delivery, result, target); err != nil {
			return deliveries, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, nil
}

func (s *DeliveryService) attempt(ctx context.Context, delivery *report.Delivery, result report.Result, target ports.DeliveryTarget) error {
	now := s.clock()
	delivery.AttemptedAt = &now
	if err := s.providers[target.Channel].Deliver(ctx, result, target.Destination); err != nil {
		delivery.Status, delivery.Error = report.DeliveryFailed, err.Error()
	} else {
		delivery.Status, delivery.Error = report.DeliverySent, ""
	}
	if err := s.repository.Update(ctx, delivery); err != nil {
		return fmt.Errorf("persist delivery status: %w", err)
	}
	return nil
}
