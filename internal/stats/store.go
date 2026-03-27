package stats

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type GameStat struct {
	RoomID          string     `bson:"_id"`
	StartedAt       time.Time  `bson:"started_at"`
	FinishedAt      *time.Time `bson:"finished_at,omitempty"`
	HasDispute      bool       `bson:"has_dispute"`
	DurationSeconds int64      `bson:"duration_seconds,omitempty"`
}

type DayStat struct {
	Date            string  `bson:"_id"`
	Launched        int64   `bson:"launched"`
	Completed       int64   `bson:"completed"`
	WithDisputes    int64   `bson:"with_disputes"`
	AvgDurationSecs float64 `bson:"avg_duration_secs"`
}

type Store struct {
	col *mongo.Collection
}

func NewStore(client *mongo.Client, dbName string) *Store {
	return &Store{col: client.Database(dbName).Collection("game_stats")}
}

func (s *Store) RecordGameStarted(ctx context.Context, roomID string, hasDispute bool) error {
	_, err := s.col.UpdateOne(
		ctx,
		bson.M{"_id": roomID},
		bson.M{"$setOnInsert": bson.M{
			"_id":        roomID,
			"started_at": time.Now().UTC(),
			"has_dispute": hasDispute,
		}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (s *Store) RecordGameFinished(ctx context.Context, roomID string) error {
	var doc GameStat
	if err := s.col.FindOne(ctx, bson.M{"_id": roomID}).Decode(&doc); err != nil {
		return err
	}
	now := time.Now().UTC()
	duration := int64(now.Sub(doc.StartedAt).Seconds())
	_, err := s.col.UpdateOne(
		ctx,
		bson.M{"_id": roomID},
		bson.M{"$set": bson.M{
			"finished_at":      now,
			"duration_seconds": duration,
		}},
	)
	return err
}

func (s *Store) GetDailyStats(ctx context.Context) ([]DayStat, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{
				{Key: "$dateToString", Value: bson.D{
					{Key: "format", Value: "%Y-%m-%d"},
					{Key: "date", Value: "$started_at"},
					{Key: "timezone", Value: "UTC"},
				}},
			}},
			{Key: "launched", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "completed", Value: bson.D{{Key: "$sum", Value: bson.D{
				{Key: "$cond", Value: bson.A{
					bson.D{{Key: "$gt", Value: bson.A{"$finished_at", nil}}},
					1, 0,
				}},
			}}}},
			{Key: "with_disputes", Value: bson.D{{Key: "$sum", Value: bson.D{
				{Key: "$cond", Value: bson.A{"$has_dispute", 1, 0}},
			}}}},
			{Key: "avg_duration_secs", Value: bson.D{{Key: "$avg", Value: bson.D{
				{Key: "$cond", Value: bson.A{
					bson.D{{Key: "$gt", Value: bson.A{"$finished_at", nil}}},
					"$duration_seconds", nil,
				}},
			}}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: -1}}}},
		{{Key: "$limit", Value: 30}},
	}

	cursor, err := s.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []DayStat
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}
