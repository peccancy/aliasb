package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/peccancy/aliasb/internal/model"
)

const roomKeyPrefix = "alias:room:"

type RoomStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRoomStore(rdb *redis.Client, ttlHours int) *RoomStore {
	return &RoomStore{rdb: rdb, ttl: time.Duration(ttlHours) * time.Hour}
}

func (s *RoomStore) Save(ctx context.Context, room *model.Room) error {
	data, err := json.Marshal(room)
	if err != nil {
		return fmt.Errorf("failed to marshal room: %w", err)
	}
	return s.rdb.Set(ctx, roomKeyPrefix+room.ID, data, s.ttl).Err()
}

func (s *RoomStore) Get(ctx context.Context, id string) (*model.Room, error) {
	data, err := s.rdb.Get(ctx, roomKeyPrefix+id).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("room not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get room: %w", err)
	}
	var room model.Room
	if err := json.Unmarshal(data, &room); err != nil {
		return nil, fmt.Errorf("failed to unmarshal room: %w", err)
	}
	return &room, nil
}

func (s *RoomStore) Delete(ctx context.Context, id string) error {
	return s.rdb.Del(ctx, roomKeyPrefix+id).Err()
}

func (s *RoomStore) Exists(ctx context.Context, id string) (bool, error) {
	n, err := s.rdb.Exists(ctx, roomKeyPrefix+id).Result()
	return n > 0, err
}

func (s *RoomStore) Count(ctx context.Context) (int64, error) {
	var count int64
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, roomKeyPrefix+"*", 100).Result()
		if err != nil {
			return 0, err
		}
		count += int64(len(keys))
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return count, nil
}
