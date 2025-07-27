package internal

import (
	"time"
)

type PaymentEndpoint = string

var (
	PaymentEndpointDefault  PaymentEndpoint = "default"
	PaymentEndpointFallback PaymentEndpoint = "fallback"
)

type PaymentRequest struct {
	CorrelationId string  `json:"correlationId"`
	Amount        float64 `json:"amount"`
}

type PaymentRequestProcessor struct {
	PaymentRequest `json:",inline"`
	RequestedAt    *string `json:"requestedAt,omitempty"`
}

func (p *PaymentRequestProcessor) UpdateRequestTime() {
	requestedAt := time.Now().UTC().Format(time.RFC3339Nano)
	p.RequestedAt = &requestedAt
}

type SummaryResponse struct {
	DefaultSummary  SummaryTotalRequestsResponse `json:"default"`
	FallbackSummary SummaryTotalRequestsResponse `json:"fallback"`
}

type SummaryTotalRequestsResponse struct {
	TotalRequests int     `json:"totalRequests"`
	TotalAmount   float64 `json:"totalAmount"`
}

type SummaryProcessorResponse struct {
	SummaryTotalRequestsResponse
	TotalFee          float64 `json:"totalFee"`
	FeePerTransaction float64 `json:"feePerTransaction"`
}

type HealthCheckResponse struct {
	Failing         bool `json:"failing"`
	MinResponseTime int  `json:"minResponseTime"`
}
