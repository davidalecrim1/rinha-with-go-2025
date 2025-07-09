package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"resty.dev/v3"
)

var (
	ErrRetriesAreOver = errors.New("retries are over")
	ErrInvalidRequest = errors.New("invalid request")
)

type PoolProcessor struct {
	Queue   chan PaymentRequestProcessor
	Workers int
}

type PaymentProcessorAdapter struct {
	client      *resty.Client
	defaultUrl  string
	fallbackUrl string
}

func NewPaymentProcessorAdapter(
	client *resty.Client,
	defaultUrl string,
	fallbackUrl string,
) *PaymentProcessorAdapter {
	return &PaymentProcessorAdapter{
		client:      client,
		defaultUrl:  defaultUrl,
		fallbackUrl: fallbackUrl,
	}
}

func (a *PaymentProcessorAdapter) Process(ctx context.Context, payment PaymentRequestProcessor) error {
	err := a.processWithRetry(
		ctx,
		payment,
		a.defaultUrl+"/payments",
		3,
		time.Millisecond*60,
		time.Millisecond*50,
		time.Millisecond*100,
	)
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrInvalidRequest) {
		return err
	}

	// slog.Debug("failed to process in the default endpoint, fallbacking to the second", "err", err)

	err = a.processWithRetry(
		ctx,
		payment,
		a.fallbackUrl+"/payments",
		5,
		time.Millisecond*60,
		time.Millisecond*100,
		time.Millisecond*1000,
	)

	// slog.Debug("dropping the request giving it wasn't processed in the fallback.")
	// TODO: Send to a channel for retry to the infinite to process more payments.

	return err
}

func (a PaymentProcessorAdapter) processWithRetry(
	ctx context.Context,
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
			select {
			case <-time.After(wait):
				// will continue the execution after the select.
			case <-ctx.Done():
				return ctx.Err()
			}
			wait = min(wait*2, maxWait)
			payment.UpdateRequestTime()
		}

		// slog.Debug("sending the request with retry", "attempt", attempt, "body", payment, "lastErr", lastErr, "url", url)

		req := a.client.R().
			SetContext(ctx).
			SetHeader("Connection", "keep-alive").
			SetHeader("Accept-Encoding", "gzip, deflate, br").
			SetTimeout(timeout).
			SetDoNotParseResponse(true).
			SetBody(payment)

		res, err := req.Post(url)
		if a.isInvalidRequest(res) {
			return ErrInvalidRequest
		}

		if !a.shouldRetry(res, err) {
			return err
		}

		lastErr = err
	}

	return fmt.Errorf("payment failed after %d attempts: %w", maxRetries+1, lastErr)
}

func (a *PaymentProcessorAdapter) isInvalidRequest(res *resty.Response) bool {
	return res.StatusCode() == 422
}

func (a *PaymentProcessorAdapter) shouldRetry(res *resty.Response, err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if res.StatusCode() >= 500 || res.StatusCode() == 429 || res.StatusCode() == 408 {
		return true
	}

	return false
}

func (a *PaymentProcessorAdapter) Summary(from, to, token string) (SummaryResponse, error) {
	resDefaultBody, err := a.summary(a.defaultUrl+"/admin/payments-summary", from, to, token)
	if err != nil {
		return SummaryResponse{}, err
	}
	resFallbackBody, err := a.summary(a.fallbackUrl+"/admin/payments-summary", from, to, token)
	if err != nil {
		return SummaryResponse{}, err
	}

	return SummaryResponse{
		DefaultSummary:  resDefaultBody,
		FallbackSummary: resFallbackBody,
	}, nil
}

func (a *PaymentProcessorAdapter) summary(url string, from, to, token string) (SummaryTotalRequestsResponse, error) {
	req := a.client.R()
	req.SetHeader("Connection", "keep-alive").
		SetHeader("Accept-Encoding", "gzip, deflate, br").
		SetHeader("X-Rinha-Token", token)

	if from != "" && to != "" {
		req.SetQueryParam("from", from)
		req.SetQueryParam("to", to)
	}

	res, err := req.Get(url)
	if err != nil {
		return SummaryTotalRequestsResponse{}, err
	}
	defer res.Body.Close()
	var resBody SummaryTotalRequestsResponse
	if err := json.NewDecoder(res.Body).Decode(&resBody); err != nil {
		return SummaryTotalRequestsResponse{}, err
	}

	return resBody, nil
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
	res, err := a.client.R().
		SetHeader("X-Rinha-Token", token).
		Post(url)
	if err != nil {
		// slog.Error("failed to purge the api", "error", err, "url", url)
		return err
	}

	if res.StatusCode() != 200 {
		return ErrInvalidRequest
	}

	return nil
}
