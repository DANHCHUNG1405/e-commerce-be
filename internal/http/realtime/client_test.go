package realtime

import (
	"context"
	"github.com/google/uuid"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeWebSocketClient(t *testing.T) {
	if os.Getenv("TEST_WEBSOCKET_CLIENT") != "1" {
		t.Skip("set TEST_WEBSOCKET_CLIENT=1 with Node.js 20.10+ to test the native WebSocket API")
	}
	_, server, tokens := setup(t)
	user := uuid.New()
	token, err := tokens.Generate(user, "customer", time.Minute)
	if err != nil {
		t.Fatal("cannot create test token")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "--experimental-websocket", filepath.Join("testdata", "client", "smoke.mjs"))
	cmd.Env = append(os.Environ(), "CHAT_TEST_URL="+server.URL, "CHAT_TEST_TOKEN="+token, "CHAT_TEST_USER="+user.String())
	if err := cmd.Run(); err != nil {
		t.Fatal("native WebSocket client compatibility test failed")
	}
}
