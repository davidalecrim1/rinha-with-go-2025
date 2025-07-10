package internal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/bytedance/sonic"
)

var (
	ErrRetriesAreOver = errors.New("retries are over")
	ErrInvalidRequest = errors.New("invalid request")
)

type PaymentProcessorAdapter struct {
	client      *http.Client
	defaultUrl  string
	fallbackUrl string
	slowQueue   chan PaymentRequestProcessor
	workers     int
}

func NewPaymentProcessorAdapter(
	client *http.Client,
	defaultUrl string,
	fallbackUrl string,
	slowQueue chan PaymentRequestProcessor,
	workers int,
) *PaymentProcessorAdapter {
	return &PaymentProcessorAdapter{
		client:      client,
		defaultUrl:  defaultUrl,
		fallbackUrl: fallbackUrl,
		slowQueue:   slowQueue,
		workers:     workers,
	}
}

func (a *PaymentProcessorAdapter) Process(payment PaymentRequestProcessor) {
	err := a.process(payment)
	if err != nil {
		a.slowQueue <- payment
	}
}

func (a *PaymentProcessorAdapter) process(payment PaymentRequestProcessor) error {
	err := a.processPaymentWithRetry(
		payment,
		a.defaultUrl+"/payments",
		5,
		time.Millisecond*20,
		time.Millisecond*500,
		time.Millisecond*2000,
	)

	if err == nil {
		return nil
	}

	slog.Debug("failed to process in the default endpoint", "err", err)

	if errors.Is(err, ErrInvalidRequest) {
		return nil
	}

	err = a.processPaymentWithRetry(
		payment,
		a.fallbackUrl+"/payments",
		3,
		time.Millisecond*20,
		time.Millisecond*1000,
		time.Millisecond*2000,
	)

	return err
}

func (a *PaymentProcessorAdapter) StartWorkers() {
	for range a.workers {
		go a.processSlowQueue()
	}

	go func() {
		for {
			slog.Info("Status of queue", "lenSlowQueue", len(a.slowQueue))
			time.Sleep(3 * time.Second)
		}
	}()
}

func (a *PaymentProcessorAdapter) processSlowQueue() {
	for payment := range a.slowQueue {
		err := a.process(payment)
		if err != nil {
			slog.Debug("failed to process payment again, sending back to the queue", "error", err)
			time.Sleep(time.Second * 2)
			a.slowQueue <- payment
		}
	}
}

func (a PaymentProcessorAdapter) processPaymentWithRetry(
	payment PaymentRequestProcessor,
	url string,
	maxRetries int,
	timeout time.Duration,
	minWait time.Duration,
	maxWait time.Duration,
) error {
	var lastErr error
	wait := minWait

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(wait)
			wait = min(wait*2, maxWait)
		}

		slog.Debug("sending the request with retry", "attempt", attempt, "body", payment, "lastErr", lastErr, "url", url)

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
		if !a.shouldRetry(res, err) {
			if a.isInvalidRequest(res) {
				return ErrInvalidRequest
			}

			return err
		}

		lastErr = err
	}

	return fmt.Errorf("payment failed after %d attempts: %w", maxRetries+1, lastErr)
}

func (a *PaymentProcessorAdapter) isInvalidRequest(res *http.Response) bool {
	return res != nil && res.StatusCode == 422
}

func (a *PaymentProcessorAdapter) shouldRetry(res *http.Response, err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	if res != nil && (res.StatusCode >= 500 ||
		res.StatusCode == 429 ||
		res.StatusCode == 408) {
		return true
	}

	return false
}

func (a *PaymentProcessorAdapter) Summary(from, to, token string) (SummaryResponse, error) {
	resDefaultBody, err := a.summary(a.defaultUrl+"/admin/payments-summary", from, to, token)
	if err != nil {
		slog.Debug("failed to get summary response from default", "err", err, "resDefaultBody", resDefaultBody)
		return SummaryResponse{}, err
	}
	resFallbackBody, err := a.summary(a.fallbackUrl+"/admin/payments-summary", from, to, token)
	if err != nil {
		slog.Debug("failed to get summary response from fallback", "err", err, "resFallbackBody", resFallbackBody)
		return SummaryResponse{}, err
	}

	return SummaryResponse{
		DefaultSummary:  resDefaultBody,
		FallbackSummary: resFallbackBody,
	}, nil
}

func (a *PaymentProcessorAdapter) summary(rawUrl string, from, to, token string) (SummaryTotalRequestsResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reqURL, err := url.Parse(rawUrl)
	if err != nil {
		return SummaryTotalRequestsResponse{}, err
	}

	query := reqURL.Query()
	if from != "" && to != "" {
		query.Set("from", from)
		query.Set("to", to)
	}
	reqURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return SummaryTotalRequestsResponse{}, err
	}

	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("X-Rinha-Token", token)

	resp, err := a.client.Do(req)
	if err != nil {
		return SummaryTotalRequestsResponse{}, err
	}
	defer resp.Body.Close()

	var parsed SummaryTotalRequestsResponse
	if err = sonic.ConfigFastest.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return SummaryTotalRequestsResponse{}, err
	}

	return parsed, nil
}

func (a *PaymentProcessorAdapter) Purge(token string) error {
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
