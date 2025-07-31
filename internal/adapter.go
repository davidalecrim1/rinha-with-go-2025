package internal

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrRetriesAreOver       = errors.New("retries are over")
	ErrInvalidRequest       = errors.New("invalid request")
	ErrUnavailableProcessor = errors.New("unavailable processor")
)

type PaymentAdapter struct {
	client      *http.Client
	redis       *redis.Client
	repo        *PaymentRepository
	defaultUrl  string
	fallbackUrl string
}

func NewPaymentAdapter(
	client *http.Client,
	redis *redis.Client,
	repo *PaymentRepository,
	defaultUrl string,
	fallbackUrl string,
) *PaymentAdapter {
	return &PaymentAdapter{
		client:      client,
		redis:       redis,
		repo:        repo,
		defaultUrl:  defaultUrl,
		fallbackUrl: fallbackUrl,
	}
}

func (a *PaymentAdapter) Process(payment []byte) {
	err := a.redis.LPush(context.Background(), PaymentProcessingQueue, payment).Err()
	if err != nil {
		slog.Error("failed to push the payment to the queue", "error", err)
	}
}

func (a *PaymentAdapter) Summary(from, to string) (SummaryResponse, error) {
	return a.repo.Summary(from, to)
}

func (a *PaymentAdapter) Purge(token string) error {
	if err := a.repo.Purge(); err != nil {
		return err
	}
	if err := a.purge(a.defaultUrl+"/admin/purge-payments", token); err != nil {
		return err
	}
	if err := a.purge(a.fallbackUrl+"/admin/purge-payments", token); err != nil {
		return err
	}

	if err := a.redis.FlushAll(context.Background()).Err(); err != nil {
		return err
	}

	return nil
}

func (a *PaymentAdapter) purge(url string, token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-Rinha-Token", token)

	res, err := a.client.Do(req)
	if err != nil {
		slog.Error("failed to purge the api", "error", err, "url", url)
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return ErrInvalidRequest
	}

	return nil
}
