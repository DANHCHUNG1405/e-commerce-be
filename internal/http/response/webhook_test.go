package response

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestWebhookSuccess(t *testing.T) {
	r := gin.New()
	r.POST("/hook", WebhookSuccess)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/hook", nil))
	var got struct {
		Envelope
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 || !got.Success || got.Error || got.StatusCode != 200 || got.Data.Msg != "Success" {
		t.Fatal("invalid provider acknowledgement", err)
	}
}
