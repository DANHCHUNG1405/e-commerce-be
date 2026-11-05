package recommendation

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/rediskey"
	"github.com/google/uuid"
)

func TestProfileReplayAndCategoryCapRedis(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	ctx := context.Background()
	client, err := database.ConnectRedis(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	user, category := uuid.New(), uuid.New()
	key := rediskey.Key("recommendation", "profile", user.String())
	defer client.Del(ctx, key)
	worker := ProfileWorker{Redis: client}
	now := time.Now().UTC()
	event := Event{InputEvent: InputEvent{EventID: uuid.New(), Type: ProductClick}, UserID: user, ReceivedAt: now, CategoryIDs: []uuid.UUID{category}}
	if err := worker.Apply(ctx, event, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := worker.Apply(ctx, event, 0, 1); err != nil {
		t.Fatal(err)
	}
	interests, err := LoadInterests(ctx, client, user, now)
	if err != nil || len(interests) != 1 || interests[0].Score < 1.9999 || interests[0].Score > 2 {
		t.Fatalf("replay changed score: %+v %v", interests, err)
	}
	for i := 2; i < 27; i++ {
		event.EventID = uuid.New()
		event.CategoryIDs = []uuid.UUID{uuid.New()}
		if err := worker.Apply(ctx, event, 0, int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	interests, err = LoadInterests(ctx, client, user, now)
	if err != nil || len(interests) != 20 {
		t.Fatalf("wanted 20 interests: %d %v", len(interests), err)
	}
	if ttl := client.TTL(ctx, key).Val(); ttl <= 0 || ttl > profileTTL {
		t.Fatalf("unexpected profile TTL: %s", ttl)
	}
}
