package api

import (
	"net/http"

	"github.com/example/e-commerce-be/internal/modules/notification"
	"github.com/gin-gonic/gin"
)

func NotificationRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, service *notification.Service) {
	a := v.Group("", auth)
	a.GET("/notifications", func(c *gin.Context) {
		page, limit, ok := Page(c)
		if !ok {
			return
		}
		data, err := service.List(c.Request.Context(), User(c), page, limit)
		Reply(c, http.StatusOK, data, err)
	})
	a.GET("/notifications/unread-count", func(c *gin.Context) {
		data, err := service.Unread(c.Request.Context(), User(c))
		Reply(c, http.StatusOK, data, err)
	})
	a.PATCH("/notifications/:id/read", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		Reply(c, http.StatusOK, nil, service.MarkRead(c.Request.Context(), User(c), id))
	})
	a.PATCH("/notifications/read-all", func(c *gin.Context) {
		Reply(c, http.StatusOK, nil, service.MarkAllRead(c.Request.Context(), User(c)))
	})
}
