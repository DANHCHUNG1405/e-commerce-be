package assistantapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/http/middleware"
	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/example/e-commerce-be/internal/modules/assistant"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Service interface {
	Create(context.Context, uuid.UUID) (assistant.Conversation, error)
	List(context.Context, uuid.UUID) ([]assistant.Conversation, error)
	Messages(context.Context, uuid.UUID, uuid.UUID) ([]assistant.Message, error)
	Send(context.Context, uuid.UUID, uuid.UUID, string) (assistant.Message, assistant.Message, error)
}

func NewRouter(tokens coreauth.TokenService, service Service, ready func(context.Context) error) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), middleware.Recovery())
	router.GET("/health", func(c *gin.Context) {
		response.Success(c, http.StatusOK, gin.H{"status": "ok", "timestamp": time.Now().UTC()})
	})
	router.GET("/ready", func(c *gin.Context) {
		if err := ready(c.Request.Context()); err != nil {
			response.Failure(c, http.StatusServiceUnavailable, "dependency unavailable")
			return
		}
		response.Success(c, http.StatusOK, gin.H{"status": "ready"})
	})
	a := router.Group("/api/v1/assistant", middleware.RequireAccessToken(tokens))
	a.POST("/conversations", func(c *gin.Context) {
		data, err := service.Create(c.Request.Context(), user(c))
		reply(c, http.StatusCreated, data, err)
	})
	a.GET("/conversations", func(c *gin.Context) {
		data, err := service.List(c.Request.Context(), user(c))
		reply(c, http.StatusOK, data, err)
	})
	a.GET("/conversations/:id/messages", func(c *gin.Context) {
		id, ok := conversationID(c)
		if !ok {
			return
		}
		data, err := service.Messages(c.Request.Context(), user(c), id)
		reply(c, http.StatusOK, data, err)
	})
	a.POST("/conversations/:id/messages", func(c *gin.Context) {
		id, ok := conversationID(c)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
		var in struct {
			Message string `json:"message" binding:"required"`
		}
		if c.ShouldBindJSON(&in) != nil {
			response.Failure(c, http.StatusBadRequest, "invalid request")
			return
		}
		question, answer, err := service.Send(c.Request.Context(), user(c), id, in.Message)
		if errors.Is(err, assistant.ErrInvalid) {
			response.Failure(c, http.StatusBadRequest, "invalid message")
			return
		}
		if errors.Is(err, assistant.ErrForbidden) {
			response.Failure(c, http.StatusNotFound, "conversation not found")
			return
		}
		if errors.Is(err, assistant.ErrConflict) {
			response.Failure(c, http.StatusConflict, "conversation changed; retry your message")
			return
		}
		if errors.Is(err, assistant.ErrRateLimited) {
			response.Failure(c, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		if err != nil {
			response.Failure(c, http.StatusServiceUnavailable, "assistant temporarily unavailable")
			return
		}
		response.Success(c, http.StatusOK, gin.H{"userMessage": question, "assistantMessage": answer})
	})
	return router
}
func user(c *gin.Context) uuid.UUID { id, _ := uuid.Parse(c.GetString("user_id")); return id }
func conversationID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		response.Failure(c, http.StatusBadRequest, "invalid conversation id")
		return uuid.Nil, false
	}
	return id, true
}
func reply(c *gin.Context, status int, data any, err error) {
	if err != nil {
		response.Failure(c, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Success(c, status, data)
}
