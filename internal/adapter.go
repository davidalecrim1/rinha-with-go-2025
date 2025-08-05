package internal

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var (
	ErrRetriesAreOver       = errors.New("retries are over")
	ErrInvalidRequest       = errors.New("invalid request")
	ErrUnavailableProcessor = errors.New("unavailable processor")
)

const (
	HealthCheckKeyDefault     = "health-check:default"
	HealthCheckKeyFallback    = "health-check:fallback"
	HealthCheckTicker         = 5 * time.Second
	MinAcceptableResponseTime = 200 // in milliseconds
)

type PaymentProcessorAdapter struct {
	tracer trace.Tracer

	httpClient           *http.Client
	healthStatusDefault  atomic.Value
	healthStatusFallback atomic.Value
	defaultUrl           string
	fallbackUrl          string

	redisClient *redis.Client
	repo        *PaymentRepository

	retryQueue chan PaymentRequestProcessor
	workers    int
}

func NewPaymentProcessorAdapter(
	tracer trace.Tracer,
	httpClient *http.Client,
	redisClient *redis.Client,
	repo *PaymentRepository,
	defaultUrl string,
	fallbackUrl string,
	retryQueue chan PaymentRequestProcessor,
	workers int,
) *PaymentProcessorAdapter {
	a := &PaymentProcessorAdapter{
		tracer: tracer,

		httpClient:  httpClient,
		defaultUrl:  defaultUrl,
		fallbackUrl: fallbackUrl,

		redisClient: redisClient,
		repo:        repo,

		retryQueue: retryQueue,
		workers:    workers,
	}

	a.healthStatusDefault.Store(HealthCheckResponse{
		Failing:         false,
		MinResponseTime: 0,
	})
	a.healthStatusFallback.Store(HealthCheckResponse{
		Failing:         false,
		MinResponseTime: 0,
	})

	return a
}

func (a *PaymentProcessorAdapter) Process(payment PaymentRequestProcessor) {
	ctx, span := a.tracer.Start(context.Background(), "adapter.Process")
	defer span.End()

	err := a.innerProcess(ctx, span, payment)
	if err != nil {
		a.retryQueue <- payment
	}
}

func (a *PaymentProcessorAdapter) innerProcess(ctx context.Context, span trace.Span, payment PaymentRequestProcessor) error {
	healthStatusDefault := a.healthStatusDefault.Load().(HealthCheckResponse)

	var err error
	if !healthStatusDefault.Failing && healthStatusDefault.MinResponseTime < MinAcceptableResponseTime {
		err = a.sendPayment(
			ctx,
			span,
			payment,
			a.defaultUrl+"/payments",
			time.Second*10,
			PaymentEndpointDefault,
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
	ctx context.Context,
	span trace.Span,
	payment PaymentRequestProcessor,
	url string,
	timeout time.Duration,
	endpoint PaymentEndpoint,
) error {
	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}()

	span.SetAttributes(
		attribute.String("payment.correlation_id", payment.CorrelationId),
	)

	payment.UpdateRequestTime()
	raw, err := sonic.ConfigFastest.Marshal(payment)
	if err != nil {
		slog.Error("failed to marshal the payment", "err", err)
		return err
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		slog.Error("failed to create the request", "err", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "keep-alive")

	res, err := a.httpClient.Do(req)
	slog.Debug("response from the processor", "res", res, "err", err)
	if res != nil {
		defer res.Body.Close()
	}
	if res != nil && res.StatusCode == 422 {
		return nil
	}
	if res != nil && res.StatusCode == 500 {
		return ErrUnavailableProcessor
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrUnavailableProcessor
	}

	if res != nil && res.StatusCode != 200 {
		slog.Error("failed to process the request", "err", err, "res", res)
		return ErrUnavailableProcessor
	}
	if err != nil || res == nil {
		slog.Error("failed to process the request", "err", err, "res", res)
		return ErrUnavailableProcessor
	}

	return a.repo.Add(
		ctx,
		PaymentProcessed{
			PaymentRequestProcessor: payment,
			Processed:               endpoint,
		})
}

func (a *PaymentProcessorAdapter) Summary(ctx context.Context, from, to string) (SummaryResponse, error) {
	return a.repo.Summary(ctx, from, to)
}

func (a *PaymentProcessorAdapter) Purge(ctx context.Context, token string) error {
	if err := a.repo.Purge(ctx); err != nil {
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

	res, err := a.httpClient.Do(req)
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

func (a *PaymentProcessorAdapter) EnableHealthCheck() {
	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for range ticker.C {
			ctx, span := a.tracer.Start(context.Background(), "adapter.EnableHealthCheckDefault")

			if err := a.storeHealthStatus(ctx, a.defaultUrl+"/payments/service-health", HealthCheckKeyDefault); err != nil {
				slog.Debug("failed to update the health check", "err", err)
			}

			span.End()
		}
	}()

	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for range ticker.C {
			ctx, span := a.tracer.Start(context.Background(), "adapter.EnableHealthCheckFallback")

			if err := a.storeHealthStatus(ctx, a.fallbackUrl+"/payments/service-health", HealthCheckKeyFallback); err != nil {
				slog.Debug("failed to update the health check", "err", err)
			}

			span.End()
		}
	}()
}

func (a *PaymentProcessorAdapter) storeHealthStatus(ctx context.Context, url string, key string) error {
	resDefault, err := a.retrieveHealth(ctx, url)
	if err != nil {
		return err
	}

	reqbody := HealthCheckResponse{
		Failing:         resDefault.Failing,
		MinResponseTime: resDefault.MinResponseTime,
	}
	rawBody, err := sonic.Marshal(reqbody)
	if err != nil {
		slog.Debug("failed to encode the json object for redis", "err", err)
		return err
	}

	if err := a.redisClient.Set(ctx, key, rawBody, 0).Err(); err != nil {
		slog.Debug("failed to save health check in redis", "err", err)
		return err
	}

	slog.Debug("updating the health check", "healthCheckStatus", reqbody, "key", key)
	return nil
}

func (a *PaymentProcessorAdapter) retrieveHealth(ctx context.Context, url string) (HealthCheckResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*1)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return HealthCheckResponse{}, err
	}

	res, err := a.httpClient.Do(req)
	if res == nil || err != nil || res.StatusCode != 200 {
		slog.Debug("failed to health check", "url", url)
		return HealthCheckResponse{}, err
	}

	var respBody HealthCheckResponse
	decoder := sonic.ConfigFastest.NewDecoder(res.Body)
	if err := decoder.Decode(&respBody); err != nil {
		slog.Debug("failed to parse the response", "url", url)
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
			slog.Info("Status of queue", "lenRetryQueue", len(a.retryQueue))
			time.Sleep(2 * time.Second)
		}
	}()

	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for range ticker.C {
			if err := a.syncHealthStatus(HealthCheckKeyDefault); err != nil {
				slog.Debug("failed update the health check", "err", err)
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for range ticker.C {
			if err := a.syncHealthStatus(HealthCheckKeyFallback); err != nil {
				slog.Debug("failed update the health check", "err", err)
			}
		}
	}()
}

func (a *PaymentProcessorAdapter) syncHealthStatus(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()

	resBody, err := a.redisClient.Get(ctx, key).Result()
	if err != nil {
		slog.Debug("failed to get the health check", "err", err)
		return err
	}

	var healthCheckStatus HealthCheckResponse
	if err := sonic.ConfigFastest.Unmarshal([]byte(resBody), &healthCheckStatus); err != nil {
		slog.Debug("failed to unmarshal the health check from redis", "err", err)
		return err
	}

	switch key {
	case HealthCheckKeyDefault:
		a.healthStatusDefault.Store(healthCheckStatus)
	case HealthCheckKeyFallback:
		a.healthStatusFallback.Store(healthCheckStatus)
	}

	return nil
}

func (a *PaymentProcessorAdapter) retryWorkers() {
	for payment := range a.retryQueue {
		time.Sleep(time.Millisecond * 10) // wait before retry
		a.Process(payment)
	}
}
