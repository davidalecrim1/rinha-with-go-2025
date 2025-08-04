package internal

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"time"

	"rinha-with-go-2025/pkg/utils"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var PaymentHashMap = "payments"

type PaymentRepository struct {
	tracer trace.Tracer
	redis  *redis.Client
}

func NewPaymentRepository(tracer trace.Tracer, redis *redis.Client) *PaymentRepository {
	return &PaymentRepository{
		tracer: tracer,
		redis:  redis,
	}
}

func (r *PaymentRepository) Add(ctx context.Context, payment PaymentProcessed) error {
	ctx, span := r.tracer.Start(ctx, "repository.Add")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	raw, err := sonic.Marshal(payment)
	if err != nil {
		slog.Error("failed to marshal payment", "err", err)
		return err
	}

	err = r.redis.HSet(ctx, PaymentHashMap, payment.CorrelationId, raw).Err()
	if err != nil {
		slog.Error("failed to save payment in redis hashmap", "err", err)
	}

	return err
}

func (r *PaymentRepository) Summary(fromStr, toStr string) (SummaryResponse, error) {
	ctx, span := r.tracer.Start(context.Background(), "repository.Summary")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	response := SummaryResponse{
		DefaultSummary: SummaryTotalRequestsResponse{
			TotalRequests: 0,
			TotalAmount:   0.0,
		},
		FallbackSummary: SummaryTotalRequestsResponse{
			TotalRequests: 0,
			TotalAmount:   0.0,
		},
	}

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

	payments, err := r.redis.HGetAll(ctx, PaymentHashMap).Result()
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
			response.DefaultSummary.TotalAmount += payment.Amount
			response.DefaultSummary.TotalRequests++
		}
		if payment.Processed == PaymentEndpointFallback {
			response.FallbackSummary.TotalAmount += payment.Amount
			response.FallbackSummary.TotalRequests++
		}
	}

	response.DefaultSummary.TotalAmount = math.Round(response.DefaultSummary.TotalAmount*100) / 100
	response.FallbackSummary.TotalAmount = math.Round(response.FallbackSummary.TotalAmount*100) / 100
	return response, nil
}

func (r *PaymentRepository) Purge() error {
	ctx, span := r.tracer.Start(context.Background(), "repository.Purge")
	defer span.End()

	err := r.redis.Del(ctx, "payments").Err()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		slog.Error("failed to delete payments hash", "err", err)
	}

	return err
}
