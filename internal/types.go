package internal

import (
	"encoding/json"
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

type RFC3339NanoTime time.Time

func (t RFC3339NanoTime) MarshalJSON() ([]byte, error) {
	ts := time.Time(t).Format(time.RFC3339Nano)
	return json.Marshal(ts)
}

func (t *RFC3339NanoTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return err
	}
	*t = RFC3339NanoTime(parsed)
	return nil
}

type PaymentRequestProcessor struct {
	PaymentRequest
	RequestedAt RFC3339NanoTime `json:"requestedAt"`
}

func (p *PaymentRequestProcessor) UpdateRequestTime() {
	p.RequestedAt = RFC3339NanoTime(time.Now().UTC())
}

type PaymentProcessed struct {
	PaymentRequestProcessor
	Processed PaymentEndpoint `json:"processed"`
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
