// Package response defines the JSON contract shared by all API handlers.
package response

import (
	"github.com/gin-gonic/gin"
	"time"
)

type Envelope struct {
	StatusCode        int     `json:"statusCode"`
	Error             bool    `json:"error"`
	ResponseTimestamp string  `json:"responseTimestamp"`
	Data              Payload `json:"data"`
}
type Payload struct {
	Msg     string `json:"msg"`
	Content any    `json:"content"`
}

func NewEnvelope(status int, message string, content any) Envelope {
	if content == nil {
		content = struct{}{}
	}
	return Envelope{StatusCode: status, Error: status >= 400,
		ResponseTimestamp: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Data:              Payload{Msg: message, Content: content}}
}
func write(c *gin.Context, status int, message string, content any) {
	c.JSON(status, NewEnvelope(status, message, content))
}
func Success(c *gin.Context, status int, content any)    { write(c, status, "Success", content) }
func Failure(c *gin.Context, status int, message string) { write(c, status, message, nil) }
func Abort(c *gin.Context, status int, message string)   { c.Abort(); Failure(c, status, message) }

// WebhookSuccess adds the top-level acknowledgement required by SePay.
// Frontend endpoints continue to use Success and the unchanged Envelope.
func WebhookSuccess(c *gin.Context) {
	c.JSON(200, struct {
		Envelope
		Success bool `json:"success"`
	}{Envelope: Envelope{StatusCode: 200, ResponseTimestamp: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), Data: Payload{Msg: "Success", Content: struct{}{}}}, Success: true})
}
