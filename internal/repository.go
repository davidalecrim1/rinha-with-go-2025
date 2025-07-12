package internal

import (
	"bytes"
	"context"
	"log/slog"
	"time"

	"rinha-with-go-2025/pkg/utils"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

var PaymentHashMap = "payments"

type PaymentRepository struct {
	ctx context.Context
	db  *redis.Client
}

func NewPaymentRepository(db *redis.Client) *PaymentRepository {
	return &PaymentRepository{
		ctx: context.Background(),
		db:  db,
	}
}

func (r *PaymentRepository) Add(payment PaymentProcessed) error {
	raw, err := sonic.Marshal(payment)
	if err != nil {
		slog.Error("failed to marshal payment", "err", err)
		return err
	}

	err = r.db.HSet(r.ctx, PaymentHashMap, payment.CorrelationId, raw).Err()
	if err != nil {
		slog.Error("failed to save payment in redis hashmap", "err", err)
	}

	return err
}

func (r *PaymentRepository) Summary(fromStr, toStr string) (SummaryResponse, error) {
	fallbackTotalRequests := 0
	fallbackTotalAmount := 0.0

	defaultTotalRequests := 0
	defaultTotalAmount := 0.0

	var from, to time.Time
	filterByTime := false
	if fromStr != "" && toStr != "" {
		var err error
		from, err = time.Parse(time.RFC3339Nano, fromStr)
		if err != nil {
			slog.Error("failed to parse the from", "err", err, "from", fromStr)
		}
		to, err = time.Parse(time.RFC3339Nano, toStr)
		if err != nil {
			slog.Error("failed to parse the to", "err", err, "to", toStr)
		}

		filterByTime = err == nil
	}

	payments, err := r.db.HGetAll(r.ctx, PaymentHashMap).Result()
	if err != nil {
		slog.Error("failed to get payments from redis hashmap", "err", err)
		return SummaryResponse{}, err
	}

	for _, v := range payments {
		var payment PaymentProcessed
		decoder := sonic.ConfigFastest.NewDecoder(bytes.NewReader([]byte(v)))
		if err := decoder.Decode(&payment); err != nil {
			slog.Error("failed to process a payment", "err", err)
			return SummaryResponse{}, err
		}

		requestedAt, err := time.Parse(time.RFC3339Nano, *payment.RequestedAt)
		if err != nil {
			slog.Error("failed to process a payment given the requestedAt parsing", "err", err)
			return SummaryResponse{}, err
		}

		if filterByTime && !utils.IsWithInRange(requestedAt, from, to) {
			continue
		}

		if payment.Processed == PaymentEndpointDefault {
			defaultTotalAmount += payment.Amount
			defaultTotalRequests++
		}
		if payment.Processed == PaymentEndpointFallback {
			fallbackTotalAmount += payment.Amount
			fallbackTotalRequests++
		}
	}

	return SummaryResponse{
		DefaultSummary: SummaryTotalRequestsResponse{
			TotalRequests: defaultTotalRequests,
			TotalAmount:   defaultTotalAmount,
		},
		FallbackSummary: SummaryTotalRequestsResponse{
			TotalRequests: fallbackTotalRequests,
			TotalAmount:   fallbackTotalAmount,
		},
	}, nil
}

func (r *PaymentRepository) Purge() error {
	err := r.db.Del(r.ctx, "payments").Err()
	if err != nil {
		slog.Error("failed to delete payments hash", "err", err)
	}

	return err
}
