package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"
	"unicode/utf8"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/chat"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

const (
	authTimeout        = 5 * time.Second
	pongTimeout        = 60 * time.Second
	pingInterval       = 25 * time.Second
	writeTimeout       = 5 * time.Second
	maxConnections     = 256
	maxUserConnections = 8
	queueSize          = 32
)

type ChatService interface {
	Active(context.Context, uuid.UUID) error
	Send(context.Context, uuid.UUID, chat.SendInput) (models.ChatMessage, error)
	Read(context.Context, uuid.UUID, uuid.UUID, int64) (models.ChatRead, error)
	Typing(context.Context, uuid.UUID, uuid.UUID, bool) error
}
type Frame struct {
	Event     string          `json:"event"`
	RequestID string          `json:"requestId,omitempty"`
	Data      json.RawMessage `json:"data"`
}
type Event struct {
	Event     string `json:"event"`
	RequestID string `json:"requestId,omitempty"`
	response.Envelope
}
type Server struct {
	tokens  coreauth.TokenService
	service ChatService
	origins map[string]bool
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	peers   map[*peer]struct{}
	pending int
	closed  bool
	wg      sync.WaitGroup
}
type peer struct {
	conn   *websocket.Conn
	user   uuid.UUID // assigned under Server.mu before being eligible for broadcasts
	ctx    context.Context
	cancel context.CancelFunc
	out    chan []byte
	done   chan struct{}
	once   sync.Once
}

func New(tokens coreauth.TokenService, origins []string) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{tokens: tokens, origins: map[string]bool{}, ctx: ctx, cancel: cancel, peers: map[*peer]struct{}{}}
	for _, o := range origins {
		s.origins[o] = true
	}
	return s
}

// Bind is startup-only, before the handler is exposed to concurrent requests.
func (s *Server) Bind(service ChatService) { s.service = service }
func (s *Server) allowed(origin string, r *http.Request) bool {
	if origin == "" {
		return true
	}
	if s.origins[origin] {
		return true
	}
	u, err := url.Parse(origin)
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return err == nil && u.Scheme == scheme && u.Host == r.Host && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serve) }
func httpError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response.NewEnvelope(status, message, nil))
}
func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if !s.allowed(r.Header.Get("Origin"), r) {
		httpError(w, 403, "forbidden origin")
		return
	}
	if r.Method != http.MethodGet || r.URL.RawQuery != "" {
		httpError(w, 400, "use GET without query parameters")
		return
	}
	if !websocket.IsWebSocketUpgrade(r) {
		httpError(w, 426, "WebSocket upgrade required")
		return
	}
	s.mu.Lock()
	if s.closed || s.service == nil || s.pending+len(s.peers) >= maxConnections {
		s.mu.Unlock()
		httpError(w, 503, "realtime unavailable")
		return
	}
	s.pending++
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()
	up := websocket.Upgrader{HandshakeTimeout: authTimeout, ReadBufferSize: 4096, WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool { return s.allowed(r.Header.Get("Origin"), r) },
		Error: func(w http.ResponseWriter, r *http.Request, status int, _ error) {
			httpError(w, status, "invalid WebSocket handshake")
		}}
	conn, err := up.Upgrade(w, r, nil)
	s.mu.Lock()
	s.pending--
	if err != nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	p := &peer{conn: conn, ctx: ctx, cancel: cancel, out: make(chan []byte, queueSize), done: make(chan struct{})}
	if s.closed {
		s.mu.Unlock()
		p.stop()
		return
	}
	s.peers[p] = struct{}{}
	s.mu.Unlock()
	defer func() { p.stop(); s.mu.Lock(); delete(s.peers, p); s.mu.Unlock() }()
	// No goroutine or subscription is created until the first frame authenticates.
	conn.SetReadLimit(12 << 10)
	_ = conn.SetReadDeadline(time.Now().Add(authTimeout))
	kind, body, err := conn.ReadMessage()
	if err != nil {
		return
	}
	var first Frame
	var auth struct {
		Token string `json:"token"`
	}
	if kind != websocket.TextMessage || !utf8.Valid(body) || json.Unmarshal(body, &first) != nil || first.Event != "auth" || json.Unmarshal(first.Data, &auth) != nil || len(auth.Token) > 8192 {
		p.closeCode(4401, "authentication required")
		return
	}
	claims, err := s.tokens.Parse(auth.Token)
	user, idErr := uuid.Parse(claims.Subject)
	if err != nil || idErr != nil || user == uuid.Nil || claims.Type != "access" || claims.ExpiresAt == nil {
		p.closeCode(4401, "invalid access token")
		return
	}
	authCtx, authCancel := context.WithTimeout(ctx, authTimeout)
	err = s.service.Active(authCtx, user)
	authCancel()
	if err != nil || !time.Now().Before(claims.ExpiresAt.Time) {
		p.closeCode(4401, "authentication failed")
		return
	}
	ready, _ := json.Marshal(Event{Event: "auth:ok", Envelope: envelope(map[string]any{"userId": user}, nil)})
	s.mu.Lock()
	count := 0
	for other := range s.peers {
		if other.user == user {
			count++
		}
	}
	if s.closed || count >= maxUserConnections {
		s.mu.Unlock()
		p.closeCode(1013, "connection limit")
		return
	}
	// Queue auth:ok before making this peer visible to Publish.
	p.out <- ready
	p.user = user
	s.mu.Unlock()
	conn.SetReadLimit(32 << 10)
	_ = conn.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(pongTimeout)) })
	writerDone := make(chan struct{})
	go func() { defer close(writerDone); p.writeLoop(claims.ExpiresAt.Time) }()
	defer func() { p.stop(); <-writerDone }()
	for {
		kind, body, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if kind != websocket.TextMessage {
			p.closeCode(1003, "text frames required")
			return
		}
		if !time.Now().Before(claims.ExpiresAt.Time) {
			p.closeCode(4401, "access token expired")
			return
		}
		var frame Frame
		if !utf8.Valid(body) || json.Unmarshal(body, &frame) != nil || frame.RequestID == "" || len(frame.RequestID) > 64 {
			p.enqueue(Event{Event: "ack", Envelope: envelope(nil, shared.ErrInvalid)})
			continue
		}
		requestCtx, done := context.WithTimeout(ctx, 5*time.Second)
		data, err := s.dispatch(requestCtx, user, frame)
		done()
		if !p.enqueue(Event{Event: "ack", RequestID: frame.RequestID, Envelope: envelope(data, err)}) {
			return
		}
	}
}
func (s *Server) dispatch(ctx context.Context, user uuid.UUID, f Frame) (data any, err error) {
	defer func() {
		if recover() != nil {
			data = nil
			err = errors.New("internal error")
		}
	}()
	switch f.Event {
	case "chat:send":
		var in chat.SendInput
		if json.Unmarshal(f.Data, &in) != nil {
			return nil, shared.ErrInvalid
		}
		return s.service.Send(ctx, user, in)
	case "chat:read":
		var in struct {
			ConversationID uuid.UUID `json:"conversationId"`
			Sequence       *int64    `json:"sequence"`
		}
		if json.Unmarshal(f.Data, &in) != nil || in.Sequence == nil {
			return nil, shared.ErrInvalid
		}
		return s.service.Read(ctx, user, in.ConversationID, *in.Sequence)
	case "chat:typing":
		var in struct {
			ConversationID uuid.UUID `json:"conversationId"`
			Typing         *bool     `json:"typing"`
		}
		if json.Unmarshal(f.Data, &in) != nil || in.Typing == nil {
			return nil, shared.ErrInvalid
		}
		return nil, s.service.Typing(ctx, user, in.ConversationID, *in.Typing)
	default:
		return nil, shared.ErrInvalid
	}
}
func (p *peer) stop() { p.once.Do(func() { p.cancel(); close(p.done); _ = p.conn.Close() }) }
func (p *peer) closeCode(code int, message string) {
	_ = p.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, message), time.Now().Add(time.Second))
	p.stop()
}
func (p *peer) enqueue(event Event) bool {
	body, err := json.Marshal(event)
	if err != nil {
		return false
	}
	return p.offer(body)
}
func (p *peer) offer(body []byte) bool {
	select {
	case <-p.done:
		return false
	default:
	}
	select {
	case p.out <- body:
		return true
	case <-p.done:
		return false
	default:
		p.stop()
		return false
	}
}

// The sole data writer also owns heartbeat and expiry; WriteControl is concurrency-safe.
func (p *peer) writeLoop(expires time.Time) {
	defer p.stop()
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	expiry := time.NewTimer(time.Until(expires))
	defer expiry.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-p.ctx.Done():
			return
		case <-expiry.C:
			p.closeCode(4401, "access token expired")
			return
		case <-ping.C:
			if err := p.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeTimeout)); err != nil {
				return
			}
		case body := <-p.out:
			if !time.Now().Before(expires) {
				p.closeCode(4401, "access token expired")
				return
			}
			_ = p.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := p.conn.WriteMessage(websocket.TextMessage, body); err != nil {
				return
			}
		}
	}
}
func (s *Server) Publish(users []uuid.UUID, event string, payload any) {
	body, err := json.Marshal(Event{Event: event, Envelope: envelope(payload, nil)})
	if err != nil {
		return
	}
	wanted := map[uuid.UUID]bool{}
	for _, id := range users {
		if id != uuid.Nil {
			wanted[id] = true
		}
	}
	s.mu.Lock()
	targets := []*peer{}
	for p := range s.peers {
		if wanted[p.user] {
			targets = append(targets, p)
		}
	}
	s.mu.Unlock()
	for _, p := range targets {
		p.offer(body)
	}
}
func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	peers := []*peer{}
	for p := range s.peers {
		peers = append(peers, p)
	}
	s.mu.Unlock()
	for _, p := range peers {
		p.stop()
	}
	s.wg.Wait()
}

var errUnauthorized = errors.New("unauthorized")

func envelope(data any, err error) response.Envelope {
	status, msg := 200, "Success"
	if err != nil {
		status, msg = 500, "internal server error"
		switch {
		case errors.Is(err, shared.ErrInvalid):
			status, msg = 400, "invalid input"
		case errors.Is(err, errUnauthorized):
			status, msg = 401, "unauthorized"
		case errors.Is(err, shared.ErrForbidden):
			status, msg = 403, "forbidden"
		case errors.Is(err, gorm.ErrRecordNotFound):
			status, msg = 404, "not found"
		case errors.Is(err, shared.ErrConflict):
			status, msg = 409, "conflict"
		case errors.Is(err, shared.ErrRateLimited):
			status, msg = 429, "rate limit exceeded"
		case errors.Is(err, shared.ErrUnavailable):
			status, msg = 503, "service unavailable"
		}
		data = nil
	}
	return response.NewEnvelope(status, msg, data)
}
