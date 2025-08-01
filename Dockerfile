FROM golang:1.24-alpine3.20 AS builder
WORKDIR /app

COPY go.* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o main ./cmd/api/main.go 

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/main .
CMD ["./main"]