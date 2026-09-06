package chat

import (
	"context"
	"github.com/example/e-commerce-be/internal/database"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/google/uuid"
	"os"
	"sync"
	"testing"
	"time"
)

type unlimited struct{}

func (unlimited) Allow(context.Context, uuid.UUID, string, int, time.Duration) error { return nil }

type capture struct {
	mu    sync.Mutex
	count int
}

func (c *capture) Publish([]uuid.UUID, string, any) { c.mu.Lock(); defer c.mu.Unlock(); c.count++ }
func TestChatIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL container")
	}
	db, err := database.Connect(dsn)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	create := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	users := make([]models.User, 3)
	for i := range users {
		users[i] = models.User{Email: uuid.NewString() + "@example.test", Password: "unused"}
		create(&users[i])
	}
	shop := models.Seller{Name: "shop", Slug: uuid.NewString(), Status: "approved"}
	create(&shop)
	create(&models.SellerMember{SellerID: shop.ID, UserID: users[1].ID, Role: "owner"})
	pub := &capture{}
	s := New(NewRepository(db), pub, unlimited{})
	ctx := context.Background()
	c, err := s.Open(ctx, users[0].ID, shop.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Open(ctx, users[0].ID, shop.ID)
	if err != nil || again.ID != c.ID {
		t.Fatal("duplicate conversation")
	}
	input := SendInput{ConversationID: c.ID, ClientMessageID: uuid.New(), Body: "hello"}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Send(ctx, users[0].ID, input); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	messages, err := s.Messages(ctx, users[1].ID, c.ID, nil, nil, 20)
	if err != nil || len(messages) != 1 || messages[0].Sequence != 1 {
		t.Fatal("duplicate messages", err)
	}
	if _, err := s.Messages(ctx, users[2].ID, c.ID, nil, nil, 20); err == nil {
		t.Fatal("outsider read messages")
	}
	if _, err := s.Send(ctx, users[2].ID, input); err == nil {
		t.Fatal("outsider sent message")
	}
	input.Body = "changed"
	if _, err := s.Send(ctx, users[0].ID, input); err == nil {
		t.Fatal("idempotency mismatch accepted")
	}
	list, err := s.List(ctx, users[1].ID, 1, 20)
	if err != nil || len(list) != 1 || list[0].UnreadCount != 1 {
		t.Fatal("wrong unread count", err)
	}
	if _, err := s.Read(ctx, users[1].ID, c.ID, 2); err == nil {
		t.Fatal("future read marker accepted")
	}
	if _, err := s.Read(ctx, users[1].ID, c.ID, 1); err != nil {
		t.Fatal(err)
	}
	r, err := s.Read(ctx, users[1].ID, c.ID, 0)
	if err != nil || r.LastSequence != 1 {
		t.Fatal("read marker moved backwards", err)
	}
	if err := db.Where("seller_id=? AND user_id=?", shop.ID, users[1].ID).Delete(&models.SellerMember{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Messages(ctx, users[1].ID, c.ID, nil, nil, 20); err == nil {
		t.Fatal("removed staff retains access")
	}
	ids, err := s.repo.Recipients(ctx, c.ID)
	if err != nil || len(ids) != 1 || ids[0] != users[0].ID {
		t.Fatal("removed staff still receives events", err)
	}
}
