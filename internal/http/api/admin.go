package api

import (
	"github.com/example/e-commerce-be/internal/modules/admin"
	"github.com/gin-gonic/gin"
)

func AdminRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, service *admin.Service) {
	a := v.Group("", auth)
	a.GET("/admin/dashboard", func(c *gin.Context) {
		d, err := service.Dashboard(c.Request.Context(), User(c))
		Reply(c, 200, d, err)
	})
}
