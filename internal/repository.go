package internal

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	PaymentSortedSetDefault  = "payments:default"
	PaymentSortedSetFallback = "payments:fallback"
)

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

	var key string
	if payment.Processed == PaymentEndpointDefault {
		key = PaymentSortedSetDefault
	} else {
		key = PaymentSortedSetFallback
	}

	requestedAt, err := time.Parse(time.RFC3339Nano, *payment.RequestedAt)
	if err != nil {
		slog.Error("failed to process a payment given the requestedAt parsing", "err", err)
		return err
	}

	err = r.redis.ZAdd(ctx, key, redis.Z{
		Score:  float64(requestedAt.UnixMilli()),
		Member: fmt.Sprintf("%s:%f", payment.CorrelationId, payment.Amount),
	}).Err()

	if err != nil {
		slog.Error("failed to save payment in redis sorted set", "err", err)
	}

	return err
}

func (r *PaymentRepository) Summary(ctx context.Context, fromStr, toStr string) (SummaryResponse, error) {
	ctx, span := r.tracer.Start(ctx, "repository.Summary")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	var from, to time.Time
	var min, max string
	if fromStr == "" || toStr == "" {
		min = "-inf"
		max = "+inf"
	} else {
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

		min = fmt.Sprintf("%d", from.UnixMilli())
		max = fmt.Sprintf("%d", to.UnixMilli())
	}

	var defaultPayments, fallbackPayments []string
	var defaultSummary, fallbackSummary SummaryTotalRequestsResponse

	pipe := r.redis.Pipeline()
	cmdDefault := pipe.ZRangeByScore(ctx, PaymentSortedSetDefault, &redis.ZRangeBy{Min: min, Max: max})
	cmdFallback := pipe.ZRangeByScore(ctx, PaymentSortedSetFallback, &redis.ZRangeBy{Min: min, Max: max})
	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		slog.Error("failed to execute pipeline", "err", err)
		return SummaryResponse{}, err
	}

	defaultPayments, err = cmdDefault.Result()
	if err != nil && err != redis.Nil {
		slog.Error("failed to get default payments", "err", err)
	}

	fallbackPayments, err = cmdFallback.Result()
	if err != nil && err != redis.Nil {
		slog.Error("failed to get fallback payments", "err", err)
	}

	for _, p := range defaultPayments {
		parts := strings.Split(p, ":")
		if len(parts) != 2 {
			continue
		}
		amount, _ := strconv.ParseFloat(parts[1], 64)
		defaultSummary.TotalAmount += amount
	}

	for _, p := range fallbackPayments {
		parts := strings.Split(p, ":")
		if len(parts) != 2 {
			continue
		}
		amount, _ := strconv.ParseFloat(parts[1], 64)
		fallbackSummary.TotalAmount += amount
	}

	response := SummaryResponse{
		DefaultSummary: SummaryTotalRequestsResponse{
			TotalRequests: len(defaultPayments),
			TotalAmount:   math.Round(defaultSummary.TotalAmount*100) / 100,
		},
		FallbackSummary: SummaryTotalRequestsResponse{
			TotalRequests: len(fallbackPayments),
			TotalAmount:   math.Round(fallbackSummary.TotalAmount*100) / 100,
		},
	}

	return response, nil
}

func (r *PaymentRepository) Purge(ctx context.Context) error {
	ctx, span := r.tracer.Start(ctx, "repository.Purge")
	defer span.End()

	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	err = r.redis.Del(ctx, PaymentSortedSetDefault, PaymentSortedSetFallback).Err()
	if err != nil {
		slog.Error("failed to delete payments sorted sets", "err", err)
	}

	return err
}
