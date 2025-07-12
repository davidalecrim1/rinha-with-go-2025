package internal

import (
	"context"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var paymentsCollection = "payments"

type PaymentProcessedDocument struct {
	CorrelationId string          `bson:"_id"`
	Amount        float64         `bson:"amount,omitempty"`
	RequestedAt   time.Time       `bson:"requestedAt,omitempty"`
	ProcessedBy   PaymentEndpoint `bson:"processed,omitempty"`
}

type PaymentRepository struct {
	db         *mongo.Database
	collection *mongo.Collection
}

func NewPaymentRepository(db *mongo.Database) *PaymentRepository {
	ctx := context.Background()

	err := db.CreateCollection(ctx, paymentsCollection)
	if err != nil {
		slog.Error("failed to create the collection", "err", err)
	}

	col := db.Collection(paymentsCollection)

	idx := mongo.IndexModel{
		Keys:    bson.D{{"requestedAt", 1}}, // ascending index
		Options: options.Index().SetName("idx_requestedAt"),
	}

	name, err := col.Indexes().CreateOne(ctx, idx)
	if err != nil {
		slog.Error("failed to create the index in the collection", "err", err)
	}

	slog.Info("the index was created in mongodb", "idxName", name)

	return &PaymentRepository{
		db:         db,
		collection: col,
	}
}

func (r *PaymentRepository) Add(payment PaymentRequestProcessor, endpoint PaymentEndpoint) error {
	t, err := time.Parse(time.RFC3339Nano, *payment.RequestedAt)
	if err != nil {
		slog.Error("failed to parse the field `requestedAt` to time.Time in mongodb", "err", err)
		return err
	}
	doc := PaymentProcessedDocument{
		CorrelationId: payment.CorrelationId,
		Amount:        payment.Amount,
		RequestedAt:   t,
		ProcessedBy:   endpoint,
	}
	_, err = r.collection.InsertOne(context.Background(), doc)
	if err != nil {
		slog.Error("failed to save payment in redis hashmap", "err", err)
	}
	return err
}

func (r *PaymentRepository) Summary(fromStr, toStr string) (SummaryResponse, error) {
	ctx := context.Background()

	var from, to time.Time
	filterByTime := false
	if fromStr != "" && toStr != "" {
		var err1 error
		var err2 error
		from, err1 = time.Parse(time.RFC3339Nano, fromStr)
		if err1 != nil {
			slog.Error("failed to parse the from", "err", err1, "from", fromStr)
		}
		to, err2 = time.Parse(time.RFC3339Nano, toStr)
		if err2 != nil {
			slog.Error("failed to parse the to", "err", err2, "to", toStr)
		}

		filterByTime = err1 == nil && err2 == nil
	}

	var filter bson.M
	if filterByTime {
		filter = bson.M{
			"requestedAt": bson.M{
				"$gte": from,
				"$lte": to,
			},
		}
	} else {
		filter = bson.M{}
	}

	opts := options.Find().SetSort(bson.D{{"requestedAt", 1}})
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return SummaryResponse{}, err
	}
	defer cursor.Close(ctx)

	var results []PaymentProcessedDocument
	if err := cursor.All(ctx, &results); err != nil {
		return SummaryResponse{}, err
	}

	response := SummaryResponse{
		DefaultSummary: SummaryTotalRequestsResponse{
			TotalRequests: 0,
			TotalAmount:   0.0,
		},
		FallbackSummary: SummaryTotalRequestsResponse{
			TotalRequests: 0,
			TotalAmount:   0.0,
		},
	}

	for _, payment := range results {
		if payment.ProcessedBy == PaymentEndpointDefault {
			response.DefaultSummary.TotalAmount += payment.Amount
			response.DefaultSummary.TotalRequests++
		} else {
			response.FallbackSummary.TotalAmount += payment.Amount
			response.FallbackSummary.TotalRequests++
		}
	}

	return response, nil
}

func (r *PaymentRepository) Purge() error {
	ctx := context.Background()

	if err := r.collection.Drop(ctx); err != nil {
		slog.Error("failed to delete payments collection", "err", err)
		return err
	}

	return nil
}
