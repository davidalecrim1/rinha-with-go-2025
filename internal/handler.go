package internal

import (
	"log/slog"
	"net/http"

	"github.com/gofiber/fiber/v2"
)

type PaymentHandler struct {
	a *PaymentProcessorAdapter
}

func NewPaymentHandler(a *PaymentProcessorAdapter) *PaymentHandler {
	return &PaymentHandler{
		a: a,
	}
}

func (h *PaymentHandler) Process(c *fiber.Ctx) error {
	var req PaymentRequest
	if err := c.BodyParser(&req); err != nil {
		slog.Error("failed to parse the request body", "error", err)
		return c.SendStatus(400)
	}

	payment := PaymentRequestProcessor{
		req,
		nil,
	}

	go h.a.Process(payment)
	return c.SendStatus(http.StatusAccepted)
}

func (h *PaymentHandler) Summary(c *fiber.Ctx) error {
	fromStr := c.Query("from")
	toStr := c.Query("to")
	tokenStr := c.Get("X-Rinha-Token")

	if tokenStr == "" {
		tokenStr = "123"
	}

	summary, err := h.a.Summary(fromStr, toStr, tokenStr)
	if err != nil {
		return c.SendStatus(500)
	}

	return c.JSON(summary)
}

func (h *PaymentHandler) Purge(c *fiber.Ctx) error {
	tokenStr := c.Get("X-Rinha-Token")

	if tokenStr == "" {
		tokenStr = "123"
	}

	if err := h.a.Purge(tokenStr); err != nil {
		return c.SendStatus(500)
	}

	return c.SendStatus(200)
}
