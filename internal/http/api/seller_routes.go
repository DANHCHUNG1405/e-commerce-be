package api

import (
	"github.com/example/e-commerce-be/internal/modules/seller"
	"github.com/gin-gonic/gin"
)

func SellerCoreRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, shops *seller.Service) {
	v.GET("/sellers/:seller", func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		data, err := shops.Public(c.Request.Context(), id)
		Reply(c, 200, data, err)
	})
	a := v.Group("", auth)
	a.POST("/sellers", func(c *gin.Context) {
		var in struct {
			Name string `json:"name" binding:"required,max=200"`
			Slug string `json:"slug" binding:"required,max=200"`
		}
		if !Bind(c, &in) {
			return
		}
		data, err := shops.Create(c.Request.Context(), User(c), in.Name, in.Slug)
		Reply(c, 201, data, err)
	})
	a.GET("/users/me/sellers", func(c *gin.Context) {
		p, l, ok := Page(c)
		if !ok {
			return
		}
		data, err := shops.Mine(c.Request.Context(), User(c), p, l)
		Reply(c, 200, data, err)
	})
	a.PATCH("/admin/sellers/:seller/status", func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		var in struct {
			Status string `json:"status" binding:"required,oneof=approved rejected suspended"`
		}
		if !Bind(c, &in) {
			return
		}
		Reply(c, 200, nil, shops.Status(c.Request.Context(), User(c), id, in.Status))
	})
}
