package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/example/e-commerce-be/internal/rediskey"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type refreshTokenStore struct {
	client *redis.Client
}

func newRefreshTokenStore(client *redis.Client) *refreshTokenStore {
	return &refreshTokenStore{client: client}
}

func (s *refreshTokenStore) Store(ctx context.Context, tokenHash string, userID uuid.UUID, ttl time.Duration) error {
	if err := s.client.Set(ctx, rediskey.Key("auth", "refresh", tokenHash), userID.String(), ttl).Err(); err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}
	return nil
}

// Consume atomically reads and deletes a refresh token, preventing reuse during rotation.
func (s *refreshTokenStore) Consume(ctx context.Context, tokenHash string, userID uuid.UUID) (bool, error) {
	storedUserID, err := s.client.GetDel(ctx, rediskey.Key("auth", "refresh", tokenHash)).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("consume refresh token: %w", err)
	}
	return storedUserID == userID.String(), nil
}

func (s *refreshTokenStore) Delete(ctx context.Context, tokenHash string) (bool, error) {
	deleted, err := s.client.Del(ctx, rediskey.Key("auth", "refresh", tokenHash)).Result()
	if err != nil {
		return false, fmt.Errorf("delete refresh token: %w", err)
	}
	return deleted == 1, nil
}
