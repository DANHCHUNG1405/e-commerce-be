package api

import (
	"github.com/example/e-commerce-be/internal/modules/chat"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"strconv"
)

func ChatRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, s *chat.Service) {
	a := v.Group("/chat", auth)
	a.POST("/conversations", func(c *gin.Context) {
		var in struct {
			SellerID uuid.UUID `json:"sellerId" binding:"required"`
		}
		if !Bind(c, &in) {
			return
		}
		d, e := s.Open(c.Request.Context(), User(c), in.SellerID)
		Reply(c, 200, d, e)
	})
	a.GET("/conversations", func(c *gin.Context) {
		p, l, ok := Page(c)
		if !ok {
			return
		}
		d, e := s.List(c.Request.Context(), User(c), p, l)
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
		d, e := s.Messages(c.Request.Context(), User(c), id, after, before, l)
		Reply(c, 200, d, e)
	})
	a.POST("/conversations/:id/messages", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		var in chat.SendInput
		if !Bind(c, &in) {
			return
		}
		in.ConversationID = id
		d, e := s.Send(c.Request.Context(), User(c), in)
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
		d, e := s.Read(c.Request.Context(), User(c), id, *in.Sequence)
		Reply(c, 200, d, e)
	})
}
