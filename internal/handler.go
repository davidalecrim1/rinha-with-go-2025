package internal

import (
	"log/slog"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type PaymentHandler struct {
	adapter *PaymentProcessorAdapter
	tracer  trace.Tracer
}

func NewPaymentHandler(adapter *PaymentProcessorAdapter, tracer trace.Tracer) *PaymentHandler {
	return &PaymentHandler{
		adapter: adapter,
		tracer:  tracer,
	}
}

func (h *PaymentHandler) RegisterRoutes(app *fiber.App) {
	app.Post("/payments", h.Process)
	app.Get("/payments-summary", h.Summary)
	app.Post("/purge-payments", h.Purge)
}

func (h *PaymentHandler) Process(c *fiber.Ctx) error {
	_, span := h.tracer.Start(c.Context(), "handler.Process")
	defer span.End()

	var req PaymentRequest
	if err := c.BodyParser(&req); err != nil {
		slog.Error("failed to parse the request body", "error", err)
		return c.SendStatus(http.StatusBadRequest)
	}

	span.SetAttributes(
		attribute.String("payment.correlation_id", req.CorrelationId),
	)

	payment := PaymentRequestProcessor{
		req,
		nil,
	}

	go h.adapter.Process(payment)
	return c.SendStatus(http.StatusAccepted)
}

func (h *PaymentHandler) Summary(c *fiber.Ctx) error {
	ctx := c.Context()

	fromStr := c.Query("from")
	toStr := c.Query("to")

	summary, err := h.adapter.Summary(ctx, fromStr, toStr)
	if err != nil {
		return c.SendStatus(http.StatusInternalServerError)
	}

	return c.JSON(summary)
}

func (h *PaymentHandler) Purge(c *fiber.Ctx) error {
	ctx := c.Context()
	tokenStr := c.Get("X-Rinha-Token")

	if tokenStr == "" {
		tokenStr = "123"
	}

	if err := h.adapter.Purge(ctx, tokenStr); err != nil {
		return c.SendStatus(http.StatusInternalServerError)
	}

	return c.SendStatus(http.StatusOK)
}
