package realtime

import (
	"context"
	"encoding/json"
	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/chat"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type stubChat struct{}

func (stubChat) Active(context.Context, uuid.UUID) error { return nil }
func (stubChat) Send(_ context.Context, user uuid.UUID, in chat.SendInput) (models.ChatMessage, error) {
	if err := in.Validate(); err != nil {
		return models.ChatMessage{}, err
	}
	return models.ChatMessage{ConversationID: in.ConversationID, SenderID: user, ClientMessageID: in.ClientMessageID, Body: in.Body, Sequence: 1}, nil
}
func (stubChat) Read(_ context.Context, u, c uuid.UUID, n int64) (models.ChatRead, error) {
	return models.ChatRead{ConversationID: c, UserID: u, LastSequence: n}, nil
}
func (stubChat) Typing(context.Context, uuid.UUID, uuid.UUID, bool) error { return nil }
func setup(t *testing.T) (*Server, *httptest.Server, coreauth.TokenService) {
	t.Helper()
	tokens := coreauth.NewTokenService("test-only-secret")
	s := New(tokens, []string{"https://allowed.example"})
	s.Bind(stubChat{})
	h := httptest.NewServer(s.Handler())
	t.Cleanup(func() { s.Close(); h.Close() })
	return s, h, tokens
}
func dial(t *testing.T, base string) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(base, "http")+"/api/v1/ws", nil)
	if err != nil {
		t.Fatal("connect failed", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}
func write(t *testing.T, c *websocket.Conn, event, request string, data any) {
	t.Helper()
	if err := c.WriteJSON(map[string]any{"event": event, "requestId": request, "data": data}); err != nil {
		t.Fatal("write failed", err)
	}
}
func read(t *testing.T, c *websocket.Conn) Event {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(4 * time.Second))
	var event Event
	if err := c.ReadJSON(&event); err != nil {
		t.Fatal("read failed", err)
	}
	return event
}
func authenticate(t *testing.T, c *websocket.Conn, token string) {
	t.Helper()
	write(t, c, "auth", "", map[string]string{"token": token})
	if got := read(t, c); got.Event != "auth:ok" || got.Error {
		t.Fatal("authentication failed")
	}
}
func expectClosed(t *testing.T, c *websocket.Conn, code int) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(7 * time.Second))
	_, _, err := c.ReadMessage()
	if err == nil {
		t.Fatal("connection stayed open")
	}
	if code != 0 && !websocket.IsCloseError(err, code) {
		t.Fatalf("unexpected close: %v", err)
	}
	if e, ok := err.(interface{ Timeout() bool }); ok && e.Timeout() {
		t.Fatal("connection did not close before deadline")
	}
}
func TestWebSocketChat(t *testing.T) {
	s, h, tokens := setup(t)
	user := uuid.New()
	token, _ := tokens.Generate(user, "customer", time.Minute)
	c := dial(t, h.URL)
	authenticate(t, c, token)
	input := chat.SendInput{ConversationID: uuid.New(), ClientMessageID: uuid.New(), Body: "Hello"}
	write(t, c, "chat:send", "request-1", input)
	ack := read(t, c)
	if ack.Event != "ack" || ack.RequestID != "request-1" || ack.StatusCode != 200 {
		t.Fatal("missing ACK")
	}
	data := ack.Data.Content.(map[string]any)
	if data["senderId"] != user.String() {
		t.Fatal("sender identity not bound to JWT")
	}
	s.Publish([]uuid.UUID{user}, "chat:message", map[string]any{"body": "reply"})
	if got := read(t, c); got.Event != "chat:message" {
		t.Fatal("missing event")
	}
	// Events addressed to another user must not reach this socket.
	s.Publish([]uuid.UUID{uuid.New()}, "private:event", nil)
	write(t, c, "chat:send", "bad", map[string]any{})
	if got := read(t, c); got.Event != "ack" || got.RequestID != "bad" || got.StatusCode != 400 {
		t.Fatal("unexpected event or invalid input accepted")
	}
	write(t, c, "join", "room", map[string]any{"userId": uuid.New()})
	if got := read(t, c); got.StatusCode != 400 {
		t.Fatal("client-controlled room accepted")
	}
}
func TestWebSocketAuthentication(t *testing.T) {
	s, h, tokens := setup(t)
	for _, kind := range []string{"invalid", "refresh", "expired", "missing auth", "binary"} {
		t.Run(kind, func(t *testing.T) {
			c := dial(t, h.URL)
			token := "invalid"
			switch kind {
			case "refresh":
				token, _ = tokens.GenerateRefresh(uuid.New(), "customer", time.Hour)
			case "expired":
				token, _ = tokens.Generate(uuid.New(), "customer", -time.Second)
			}
			if kind == "binary" {
				_ = c.WriteMessage(websocket.BinaryMessage, []byte("{}"))
			} else if kind == "missing auth" {
				write(t, c, "chat:send", "x", map[string]any{})
			} else {
				write(t, c, "auth", "", map[string]string{"token": token})
			}
			expectClosed(t, c, 4401)
		})
	}
	c := dial(t, h.URL)
	token, _ := tokens.Generate(uuid.New(), "customer", 2*time.Second)
	authenticate(t, c, token)
	expectClosed(t, c, 4401)
	// Closing twice is safe, closes unauthenticated connections and rejects new ones.
	pending := dial(t, h.URL)
	s.Close()
	s.Close()
	expectClosed(t, pending, 0)
}
func TestWebSocketOriginAndHTTP(t *testing.T) {
	s, _, _ := setup(t)
	for _, tc := range []struct {
		origin, path string
		want         int
	}{{"https://evil.example", "/api/v1/ws", 403}, {"https://allowed.example", "/api/v1/ws", 426}, {"", "/api/v1/ws?token=not-a-real-token", 400}, {"", "/api/v1/ws", 426}} {
		req := httptest.NewRequest("GET", "http://backend.example"+tc.path, nil)
		req.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("unexpected HTTP status %d", w.Code)
		}
		var e Event
		if json.Unmarshal(w.Body.Bytes(), &e) != nil || e.StatusCode != tc.want || !e.Error {
			t.Fatal("HTTP error not enveloped")
		}
	}
	if s.allowed("https://backend.example", httptest.NewRequest(http.MethodGet, "http://backend.example", nil)) {
		t.Fatal("scheme mismatch accepted")
	}
}
func TestWebSocketLimits(t *testing.T) {
	_, h, tokens := setup(t)
	user := uuid.New()
	token, _ := tokens.Generate(user, "customer", time.Minute)
	for i := 0; i < maxUserConnections; i++ {
		c := dial(t, h.URL)
		authenticate(t, c, token)
	}
	extra := dial(t, h.URL)
	write(t, extra, "auth", "", map[string]string{"token": token})
	expectClosed(t, extra, 1013)
	c := dial(t, h.URL)
	token, _ = tokens.Generate(uuid.New(), "customer", time.Minute)
	authenticate(t, c, token)
	_ = c.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", 33<<10)))
	expectClosed(t, c, 1009)
}
func TestWebSocketAuthTimeout(t *testing.T) {
	_, h, _ := setup(t)
	c := dial(t, h.URL)
	expectClosed(t, c, 0)
}
func TestEnvelope(t *testing.T) {
	e := envelope(nil, shared.ErrForbidden)
	if e.StatusCode != 403 || !e.Error {
		t.Fatal("unsafe error envelope")
	}
}

func TestBoundedWriterQueue(t *testing.T) {
	_, h, _ := setup(t)
	conn := dial(t, h.URL)
	ctx, cancel := context.WithCancel(context.Background())
	p := &peer{conn: conn, ctx: ctx, cancel: cancel, out: make(chan []byte, 1), done: make(chan struct{})}
	if !p.offer([]byte("first")) {
		t.Fatal("empty queue rejected event")
	}
	if p.offer([]byte("second")) {
		t.Fatal("full queue accepted event")
	}
	select {
	case <-p.done:
	default:
		t.Fatal("slow client not disconnected")
	}
	if p.offer([]byte("third")) {
		t.Fatal("closed queue accepted event")
	}
}
