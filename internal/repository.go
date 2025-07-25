package internal

import (
	"context"
	"log/slog"
	"math"
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
		Keys: bson.D{
			{"requestedAt", 1},
			{"processed", 1},
		},
		Options: options.Index().SetName("idx_requestedAt_processed"),
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

	var matchStage bson.M
	if filterByTime {
		matchStage = bson.M{
			"$match": bson.M{
				"requestedAt": bson.M{
					"$gte": from,
					"$lte": to,
				},
			},
		}
	} else {
		matchStage = bson.M{"$match": bson.M{}}
	}

	groupStage := bson.M{
		"$group": bson.M{
			"_id":           "$processed",
			"totalRequests": bson.M{"$sum": 1},
			"totalAmount":   bson.M{"$sum": "$amount"},
		},
	}

	cursor, err := r.collection.Aggregate(ctx, []bson.M{matchStage, groupStage})
	if err != nil {
		return SummaryResponse{}, err
	}
	defer cursor.Close(ctx)

	response := SummaryResponse{}
	for cursor.Next(ctx) {
		var result struct {
			ID            string  `bson:"_id"`
			TotalRequests int     `bson:"totalRequests"`
			TotalAmount   float64 `bson:"totalAmount"`
		}

		if err := cursor.Decode(&result); err != nil {
			continue
		}

		if result.ID == PaymentEndpointDefault {
			response.DefaultSummary.TotalRequests = result.TotalRequests
			response.DefaultSummary.TotalAmount = result.TotalAmount
		} else {
			response.FallbackSummary.TotalRequests = result.TotalRequests
			response.FallbackSummary.TotalAmount = result.TotalAmount
		}
	}

	response.DefaultSummary.TotalAmount = math.Round(response.DefaultSummary.TotalAmount*100) / 100
	response.FallbackSummary.TotalAmount = math.Round(response.FallbackSummary.TotalAmount*100) / 100

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
