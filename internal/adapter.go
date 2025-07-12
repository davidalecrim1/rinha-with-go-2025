package internal

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

var (
	ErrRetriesAreOver       = errors.New("retries are over")
	ErrInvalidRequest       = errors.New("invalid request")
	ErrUnavailableProcessor = errors.New("unavailable processor")
)

var HealthCheckKey = "health-check"

type PaymentProcessorAdapter struct {
	client       *http.Client
	db           *redis.Client
	repo         *PaymentRepository
	healthStatus *HealthCheckStatus
	mu           sync.RWMutex
	defaultUrl   string
	fallbackUrl  string
	retryQueue   chan PaymentRequestProcessor
	workers      int
}

func NewPaymentProcessorAdapter(
	client *http.Client,
	db *redis.Client,
	repo *PaymentRepository,
	defaultUrl string,
	fallbackUrl string,
	retryQueue chan PaymentRequestProcessor,
	workers int,
) *PaymentProcessorAdapter {
	return &PaymentProcessorAdapter{
		client: client,
		db:     db,
		repo:   repo,
		healthStatus: &HealthCheckStatus{
			Default: HealthCheckResponse{
				Failing:         false,
				MinResponseTime: 0,
			},
			Fallback: HealthCheckResponse{
				Failing:         false,
				MinResponseTime: 0,
			},
		},
		defaultUrl:  defaultUrl,
		fallbackUrl: fallbackUrl,
		retryQueue:  retryQueue,
		workers:     workers,
	}
}

func (a *PaymentProcessorAdapter) Process(payment PaymentRequestProcessor) {
	err := a.innerProcess(payment)
	if err != nil {
		a.retryQueue <- payment
	}
}

func (a *PaymentProcessorAdapter) innerProcess(payment PaymentRequestProcessor) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var err error
	if !a.healthStatus.Default.Failing && a.healthStatus.Default.MinResponseTime < 100 {
		err = a.sendPayment(
			payment,
			a.defaultUrl+"/payments",
			time.Millisecond*80,
			PaymentEndpointDefault,
		)
	} else if !a.healthStatus.Fallback.Failing && a.healthStatus.Fallback.MinResponseTime < 100 {
		err = a.sendPayment(
			payment,
			a.fallbackUrl+"/payments",
			time.Millisecond*80,
			PaymentEndpointFallback,
		)
	} else {
		return ErrUnavailableProcessor
	}

	if errors.Is(err, ErrInvalidRequest) {
		return nil
	}

	return err
}

func (a *PaymentProcessorAdapter) sendPayment(
	payment PaymentRequestProcessor,
	url string,
	timeout time.Duration,
	endpoint PaymentEndpoint,
) error {
	slog.Debug("sending the request", "body", payment, "url", url)

	payment.UpdateRequestTime()
	reqBody, err := sonic.ConfigFastest.Marshal(payment)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "keep-alive")

	res, err := a.client.Do(req)
	slog.Debug("response from api", "url", url, "res", res)
	if res != nil && res.StatusCode == 422 {
		return nil
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrUnavailableProcessor
	}

	if res != nil && (res.StatusCode >= 500 ||
		res.StatusCode == 429 ||
		res.StatusCode == 408) {
		return ErrUnavailableProcessor
	}

	err = a.repo.Add(PaymentProcessed{
		payment,
		endpoint,
	})

	return err
}

func (a *PaymentProcessorAdapter) Summary(from, to string) (SummaryResponse, error) {
	return a.repo.Summary(from, to)
}

func (a *PaymentProcessorAdapter) Purge(token string) error {
	if err := a.repo.Purge(); err != nil {
		return err
	}
	if err := a.purge(a.defaultUrl+"/admin/purge-payments", token); err != nil {
		return err
	}
	if err := a.purge(a.fallbackUrl+"/admin/purge-payments", token); err != nil {
		return err
	}

	return nil
}

func (a *PaymentProcessorAdapter) purge(url string, token string) error {
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

func (a *PaymentProcessorAdapter) EnableHealthCheck(should string) {
	if should != "true" {
		return
	}

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			resDefault, err := a.healthCheckEndpoint(a.defaultUrl + "/payments/service-health")
			if err != nil {
				continue
			}
			resFallback, err := a.healthCheckEndpoint(a.fallbackUrl + "/payments/service-health")
			if err != nil {
				continue
			}

			reqbody := HealthCheckStatus{
				resDefault,
				resFallback,
			}
			rawBody, err := sonic.Marshal(reqbody)
			if err != nil {
				slog.Debug("failed to encode the json object for redis", "err", err)
				continue
			}
			if a.db.Set(context.Background(), HealthCheckKey, rawBody, 0).Err() != nil {
				slog.Debug("failed to save health check in redis")
				continue
			}

			slog.Debug("updating the", "healthCheckStatus", reqbody)
		}
	}()
}

func (a *PaymentProcessorAdapter) healthCheckEndpoint(url string) (HealthCheckResponse, error) {
	res, err := a.client.Get(url)
	if res == nil || err != nil || res.StatusCode != 200 {
		slog.Error("failed to health check", "url", url)
		return HealthCheckResponse{}, err
	}

	var respBody HealthCheckResponse
	decoder := sonic.ConfigFastest.NewDecoder(res.Body)
	if err := decoder.Decode(&respBody); err != nil {
		slog.Error("failed to parse the response", "url", url)
		return HealthCheckResponse{}, err
	}

	return respBody, nil
}

func (a *PaymentProcessorAdapter) StartWorkers() {
	for range a.workers {
		go a.retryWorkers()
	}

	go func() {
		for {
			slog.Debug("Status of queue", "lenRetryQueue", len(a.retryQueue))
			time.Sleep(3 * time.Second)
		}
	}()

	go func() {
		ticker := time.NewTicker(time.Second * 5)
		defer ticker.Stop()

		for range ticker.C {
			res := a.db.Get(context.Background(), HealthCheckKey)
			if res.Err() != nil {
				slog.Debug("failed update the health check", "err", res.Err())
				continue
			}

			resBody, err := res.Result()
			if err != nil {
				slog.Debug("failed update the health check", "err", res.Err())
				continue

			}

			var healthCheckStatus *HealthCheckStatus
			if err := sonic.ConfigFastest.Unmarshal([]byte(resBody), &healthCheckStatus); err != nil {
				slog.Debug("failed update the health check", "err", err)
				continue
			}

			a.mu.Lock()
			a.healthStatus = healthCheckStatus
			a.mu.Unlock()
		}
	}()
}

func (a *PaymentProcessorAdapter) retryWorkers() {
	for payment := range a.retryQueue {
		time.Sleep(time.Millisecond * 500)
		a.Process(payment)
	}
}
