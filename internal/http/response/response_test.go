package response

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		content any
		want    string
	}{
		{"object", 200, map[string]string{"name": "test"}, `{"name":"test"}`},
		{"created", 201, map[string]string{"id": "example"}, `{"id":"example"}`},
		{"empty", 200, nil, `{}`},
		{"list", 200, []string{}, `[]`},
		{"error", 409, nil, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			if tc.status >= 400 {
				Failure(c, tc.status, "conflict")
			} else {
				Success(c, tc.status, tc.content)
			}
			var result struct {
				StatusCode int    `json:"statusCode"`
				Error      bool   `json:"error"`
				Timestamp  string `json:"responseTimestamp"`
				Data       struct {
					Msg     string          `json:"msg"`
					Content json.RawMessage `json:"content"`
				} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.StatusCode != tc.status || w.Code != tc.status || result.Error != (tc.status >= 400) {
				t.Fatal("status mismatch")
			}
			if string(result.Data.Content) != tc.want {
				t.Fatalf("unexpected content %s", result.Data.Content)
			}
			wantMsg := "Success"
			if tc.status >= 400 {
				wantMsg = "conflict"
			}
			if result.Data.Msg != wantMsg {
				t.Fatal("message mismatch")
			}
			if len(result.Timestamp) != 24 {
				t.Fatal("timestamp must have fixed millisecond precision")
			}
			stamp, err := time.Parse("2006-01-02T15:04:05.000Z", result.Timestamp)
			if err != nil || time.Since(stamp) > time.Minute {
				t.Fatal("invalid UTC timestamp")
			}
		})
	}
}
