package internal

import (
	"context"
	"encoding/json"
	"errors"
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
	req := a.client.R().
		SetContext(ctx).
		SetTimeout(time.Millisecond*200).
		SetRetryCount(3).
		SetRetryWaitTime(10*time.Millisecond).
		SetRetryMaxWaitTime(50*time.Millisecond).
		SetHeader("Connection", "keep-alive").
		SetHeader("Accept-Encoding", "gzip, deflate, br").
		SetBody(payment).
		SetDoNotParseResponse(true)

	err := a.sendPayment(req, a.defaultUrl+"/payments", payment)
	if err == nil {
		return nil
	}

	req = a.client.R().
		SetTimeout(time.Millisecond*200).
		SetRetryCount(3).
		SetRetryWaitTime(100*time.Millisecond).
		SetRetryMaxWaitTime(1*time.Second).
		SetHeader("Connection", "keep-alive").
		SetHeader("Accept-Encoding", "gzip, deflate, br").
		SetBody(payment).
		SetDoNotParseResponse(true)

	err = a.sendPayment(req, a.fallbackUrl+"/payments", payment)
	return err
}

func (a *PaymentProcessorAdapter) sendPayment(req *resty.Request, url string, payment PaymentRequestProcessor) error {
	res, err := req.Post(url)
	if res.StatusCode() >= 400 && res.StatusCode() < 499 {
		// slog.Debug("The request is invalid", "StatusCode", res.StatusCode(), "correlationId", payment.CorrelationId)
		return nil
	}

	if res.StatusCode() >= 500 {
		return ErrRetriesAreOver
	}

	return err
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
