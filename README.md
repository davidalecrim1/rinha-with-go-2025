# Rinha de Backend (2025)

Rinha de Backend is a backend development challenge focused on performance, efficiency, and creativity. The 2025 edition has the goal is to create a solution capable of brokering payment requests between clients and two payment processing services (Payment Processors), always seeking the lowest transaction fee and the highest number of processed payments. This has been done in Go.

## About the Challenge

You must develop a backend that receives payment requests and forwards them to two external services: the Payment Processor Default (lower fee) and the Payment Processor Fallback (higher fee, used only when the default is unavailable). The Payment Processor services may experience instabilities, such as slow responses or unavailability (HTTP 5XX). Your solution should be resilient and efficient, always prioritizing the lowest cost.

The Payment Processor source code is available at:  
https://github.com/zanfranceschi/rinha-de-backend-2025-payment-processor