package internal

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

var (
	PaymentDefaultSortedSet  = "payments:default"
	PaymentFallbackSortedSet = "payments:fallback"
)

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
	raw, err := sonic.ConfigFastest.Marshal(payment)
	if err != nil {
		slog.Error("failed to marshal payment", "err", err)
		return err
	}

	requestedAt, err := time.Parse(time.RFC3339Nano, *payment.RequestedAt)
	if err != nil {
		slog.Error("failed to parse requested at", "err", err)
		return err
	}

	z := redis.Z{
		Score:  float64(requestedAt.UnixMilli()),
		Member: raw,
	}

	var key string
	if payment.Processed == PaymentEndpointDefault {
		key = PaymentDefaultSortedSet
	} else {
		key = PaymentFallbackSortedSet
	}

	if err := r.db.ZAdd(r.ctx, key, z).Err(); err != nil {
		slog.Error("failed to save payment in redis sorted set", "err", err)
		return err
	}

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
		DefaultSummary:  r.calculateSummary(PaymentDefaultSortedSet, from, to, filterByTime),
		FallbackSummary: r.calculateSummary(PaymentFallbackSortedSet, from, to, filterByTime),
	}

	return response, nil
}

func (r *PaymentRepository) calculateSummary(key string, from, to time.Time, filterByTime bool) SummaryTotalRequestsResponse {
	var payments []string
	var err error

	if filterByTime {
		opt := &redis.ZRangeBy{
			Min: fmt.Sprintf("%d", from.UnixMilli()),
			Max: fmt.Sprintf("%d", to.UnixMilli()-1), // -1 because needs to be less than, not equal
		}
		payments, err = r.db.ZRangeByScore(r.ctx, key, opt).Result()
	} else {
		payments, err = r.db.ZRange(r.ctx, key, 0, -1).Result()
	}

	if err != nil {
		slog.Error("failed to get payments from redis sorted set", "err", err, "key", key)
		return SummaryTotalRequestsResponse{}
	}

	summary := SummaryTotalRequestsResponse{}
	for _, v := range payments {
		var payment PaymentProcessed
		decoder := sonic.ConfigFastest.NewDecoder(bytes.NewReader([]byte(v)))
		if err := decoder.Decode(&payment); err != nil {
			slog.Error("failed to process a payment", "err", err)
			continue
		}
		summary.TotalAmount += payment.Amount
		summary.TotalRequests++
	}

	summary.TotalAmount = math.Round(summary.TotalAmount*100) / 100
	return summary
}

func (r *PaymentRepository) Purge() error {
	err := r.db.Del(r.ctx, PaymentDefaultSortedSet, PaymentFallbackSortedSet).Err()
	if err != nil {
		slog.Error("failed to delete payments sorted sets", "err", err)
	}

	return err
}
