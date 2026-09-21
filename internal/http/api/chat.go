package api

import (
	"context"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"strconv"
)

type ChatService interface {
	Open(context.Context, string, uuid.UUID) (models.ChatConversation, error)
	List(context.Context, string, int, int) ([]models.ChatConversationView, error)
	Messages(context.Context, string, uuid.UUID, *int64, *int64, int) ([]models.ChatMessage, error)
	Send(context.Context, string, uuid.UUID, uuid.UUID, string) (models.ChatMessage, error)
	Read(context.Context, string, uuid.UUID, int64) (models.ChatRead, error)
}

func ChatRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, s ChatService) {
	a := v.Group("/chat", auth)
	a.POST("/conversations", func(c *gin.Context) {
		var in struct {
			SellerID uuid.UUID `json:"sellerId" binding:"required"`
		}
		if !Bind(c, &in) {
			return
		}
		d, e := s.Open(c.Request.Context(), c.GetHeader("Authorization"), in.SellerID)
		Reply(c, 200, d, e)
	})
	a.GET("/conversations", func(c *gin.Context) {
		p, l, ok := Page(c)
		if !ok {
			return
		}
		d, e := s.List(c.Request.Context(), c.GetHeader("Authorization"), p, l)
		Reply(c, 200, d, e)
	})
	a.GET("/conversations/:id/messages", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		_, l, ok := Page(c)
		if !ok {
			return
		}
		var after, before *int64
		for name, dst := range map[string]**int64{"after": &after, "before": &before} {
			if raw, exists := c.GetQuery(name); exists {
				n, e := strconv.ParseInt(raw, 10, 64)
				if e != nil {
					Reply(c, 400, nil, shared.ErrInvalid)
					return
				}
				*dst = &n
			}
		}
		d, e := s.Messages(c.Request.Context(), c.GetHeader("Authorization"), id, after, before, l)
		Reply(c, 200, d, e)
	})
	a.POST("/conversations/:id/messages", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		var in struct {
			ClientMessageID uuid.UUID `json:"clientMessageId" binding:"required"`
			Body            string    `json:"body" binding:"required"`
		}
		if !Bind(c, &in) {
			return
		}
		d, e := s.Send(c.Request.Context(), c.GetHeader("Authorization"), id, in.ClientMessageID, in.Body)
		Reply(c, 200, d, e)
	})
	a.PUT("/conversations/:id/read", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		var in struct {
			Sequence *int64 `json:"sequence" binding:"required"`
		}
		if !Bind(c, &in) {
			return
		}
		d, e := s.Read(c.Request.Context(), c.GetHeader("Authorization"), id, *in.Sequence)
		Reply(c, 200, d, e)
	})
}
