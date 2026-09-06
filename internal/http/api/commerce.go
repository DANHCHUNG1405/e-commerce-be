package api

import (
	"github.com/example/e-commerce-be/internal/modules/order"
	"github.com/example/e-commerce-be/internal/modules/review"
	"github.com/example/e-commerce-be/internal/modules/wishlist"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func Commerce(v *gin.RouterGroup, auth gin.HandlerFunc, orders *order.Service, reviews *review.Service, wishes *wishlist.Service) {
	v.GET("/products/:id/reviews", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		p, l, ok := Page(c)
		if !ok {
			return
		}
		d, e := reviews.List(c.Request.Context(), id, p, l)
		Reply(c, 200, d, e)
	})
	a := v.Group("", auth)
	a.GET("/sellers/:seller/orders", func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		p, l, ok := Page(c)
		if !ok {
			return
		}
		d, e := orders.SellerOrders(c.Request.Context(), User(c), id, p, l, c.Query("status"))
		Reply(c, 200, d, e)
	})
	a.PATCH("/sellers/:seller/orders/:id/status", func(c *gin.Context) {
		sid, ok := ID(c, "seller")
		if !ok {
			return
		}
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		var in struct {
			Status   string `json:"status" binding:"required,oneof=confirmed shipping delivered"`
			Carrier  string `json:"carrier" binding:"max=100"`
			Tracking string `json:"trackingNumber" binding:"max=200"`
		}
		if !Bind(c, &in) {
			return
		}
		Reply(c, 200, nil, orders.Fulfill(c.Request.Context(), User(c), sid, id, in.Status, in.Carrier, in.Tracking))
	})
	a.POST("/reviews", func(c *gin.Context) {
		var in struct {
			OrderItemID uuid.UUID `json:"orderItemId" binding:"required"`
			Rating      int       `json:"rating" binding:"required,min=1,max=5"`
			Comment     string    `json:"comment" binding:"max=5000"`
		}
		if !Bind(c, &in) {
			return
		}
		d, e := reviews.Create(c.Request.Context(), User(c), in.OrderItemID, in.Rating, in.Comment)
		Reply(c, 201, d, e)
	})
	a.DELETE("/admin/reviews/:id", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		Reply(c, 200, nil, reviews.Hide(c.Request.Context(), User(c), id))
	})
	a.GET("/wishlist", func(c *gin.Context) {
		p, l, ok := Page(c)
		if !ok {
			return
		}
		d, e := wishes.List(c.Request.Context(), User(c), p, l)
		Reply(c, 200, d, e)
	})
	a.PUT("/wishlist/items/:product", func(c *gin.Context) {
		id, ok := ID(c, "product")
		if !ok {
			return
		}
		Reply(c, 200, nil, wishes.Put(c.Request.Context(), User(c), id, false))
	})
	a.DELETE("/wishlist/items/:product", func(c *gin.Context) {
		id, ok := ID(c, "product")
		if !ok {
			return
		}
		Reply(c, 200, nil, wishes.Put(c.Request.Context(), User(c), id, true))
	})
}
