package main

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

type PaymentHandler struct {
	a *PaymentProcessorAdapter
}

/*
POST /payments

	{
	    "correlationId": "4a7901b8-7d26-4d9d-aa19-4dc1c7cf60b3",
	    "amount": 19.90
	}
*/
func (h *PaymentHandler) Process(c *fiber.Ctx) error {
	var req PaymentRequest
	if err := c.BodyParser(&req); err != nil {
		// slog.Error("failed to parse the request body", "error", err)
		return c.SendStatus(400)
	}

	processReq := PaymentRequestProcessor{
		req,
		time.Now().UTC().Format(time.RFC3339),
	}
	if err := h.a.Process(processReq); err != nil {
		return c.SendStatus(500)
	}

	return c.SendStatus(200)
}

/*
GET /payments-summary?from=2020-07-10T12:34:56.000Z&to=2020-07-10T12:35:56.000Z

HTTP 200 - Ok
{
    "default" : {
        "totalRequests": 43236,
        "totalAmount": 415542345.98
    },
    "fallback" : {
        "totalRequests": 423545,
        "totalAmount": 329347.34
    }
}
*/

func (h *PaymentHandler) Summary(c *fiber.Ctx) error {
	fromStr := c.Query("from")
	toStr := c.Query("to")

	summary, err := h.a.Summary(fromStr, toStr)
	if err != nil {
		return c.SendStatus(500)
	}

	return c.JSON(summary)
}

/*
GET /purge-payments
Just created because it's requested in the K6 script.
*/
func (h *PaymentHandler) Purge(c *fiber.Ctx) error {
	return c.SendStatus(200)
}
