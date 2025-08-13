package internal

import (
	"log/slog"
	"math"
	"sync"
	"time"
)

var (
	PaymentDefaultSortedSet  = "payments:default"
	PaymentFallbackSortedSet = "payments:fallback"
)

type PaymentRepository struct {
	mu   sync.Mutex
	data map[PaymentEndpoint][]PaymentProcessed
}

func NewPaymentRepository(cfg *Config) *PaymentRepository {
	data := make(map[PaymentEndpoint][]PaymentProcessed)
	data[PaymentEndpointDefault] = make([]PaymentProcessed, 0, cfg.InitalDatabaseCap)
	data[PaymentEndpointFallback] = make([]PaymentProcessed, 0, cfg.InitalDatabaseCap)

	return &PaymentRepository{
		mu:   sync.Mutex{},
		data: data,
	}
}

func (r *PaymentRepository) Add(payment PaymentProcessed) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[payment.Processed] = append(r.data[payment.Processed], payment)
	return nil
}

func (r *PaymentRepository) Summary(fromStr, toStr string) (SummaryResponse, error) {
	var from, to time.Time
	filterByTime := fromStr != "" && toStr != ""

	var err error
	if filterByTime {
		from, err = time.Parse(time.RFC3339Nano, fromStr)
		if err != nil {
			slog.Error("failed to parse the from", "err", err, "from", fromStr)
			return SummaryResponse{}, err
		}
		to, err = time.Parse(time.RFC3339Nano, toStr)
		if err != nil {
			slog.Error("failed to parse the to", "err", err, "to", toStr)
			return SummaryResponse{}, err
		}
	}

	response := SummaryResponse{
		DefaultSummary:  r.calculateSummary(PaymentEndpointDefault, from, to, filterByTime),
		FallbackSummary: r.calculateSummary(PaymentEndpointFallback, from, to, filterByTime),
	}

	return response, nil
}

func (r *PaymentRepository) calculateSummary(endpoint PaymentEndpoint, from, to time.Time, filterByTime bool) SummaryTotalRequestsResponse {
	summary := SummaryTotalRequestsResponse{}

	r.mu.Lock()
	data := r.data[endpoint]
	r.mu.Unlock()

	for _, payment := range data {
		if filterByTime {
			requestedAt := time.Time(payment.RequestedAt)

			isBeforeFrom := requestedAt.Before(from) // strictly less than from
			isOnOrAfterTo := !requestedAt.Before(to) // greater than or equal to to

			if isBeforeFrom || isOnOrAfterTo {
				continue
			}
		}
		summary.TotalAmount += payment.Amount
		summary.TotalRequests++
	}

	summary.TotalAmount = math.Round(summary.TotalAmount*100) / 100
	return summary
}

func (r *PaymentRepository) Purge() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = make(map[string][]PaymentProcessed)
	return nil
}
