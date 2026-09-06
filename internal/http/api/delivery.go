package api

import (
	"github.com/example/e-commerce-be/internal/modules/shipping"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func DeliveryRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, s *shipping.Service) {
	a := v.Group("", auth)
	a.POST("/drivers/apply", func(c *gin.Context) {
		var in shipping.Application
		if !Bind(c, &in) {
			return
		}
		d, e := s.Apply(c.Request.Context(), User(c), in)
		Reply(c, 200, d, e)
	})
	a.GET("/drivers/me", func(c *gin.Context) { d, e := s.Me(c.Request.Context(), User(c)); Reply(c, 200, d, e) })
	a.GET("/admin/drivers", func(c *gin.Context) {
		p, l, ok := Page(c)
		if !ok {
			return
		}
		d, e := s.Drivers(c.Request.Context(), User(c), c.Query("status"), p, l)
		Reply(c, 200, d, e)
	})
	a.PATCH("/admin/drivers/:id/status", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		var in struct {
			Status string `json:"status"`
		}
		if !Bind(c, &in) {
			return
		}
		Reply(c, 200, nil, s.Approve(c.Request.Context(), User(c), id, in.Status))
	})
	a.POST("/sellers/:seller/orders/:id/shipment", func(c *gin.Context) {
		shop, ok := ID(c, "seller")
		if !ok {
			return
		}
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		d, e := s.Create(c.Request.Context(), User(c), shop, id)
		Reply(c, 200, d, e)
	})
	a.PUT("/admin/shipments/:id/driver", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		var in struct {
			DriverID uuid.UUID `json:"driverId"`
		}
		if !Bind(c, &in) {
			return
		}
		Reply(c, 200, nil, s.Assign(c.Request.Context(), User(c), id, in.DriverID))
	})
	a.POST("/admin/shipments/:id/settle-cod", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		Reply(c, 200, nil, s.Settle(c.Request.Context(), User(c), id))
	})
	list := func(admin bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			p, l, ok := Page(c)
			if !ok {
				return
			}
			d, e := s.List(c.Request.Context(), User(c), admin, c.Query("status"), p, l)
			Reply(c, 200, d, e)
		}
	}
	a.GET("/admin/shipments", list(true))
	a.GET("/drivers/me/shipments", list(false))
	a.GET("/shipments/:id", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		d, e := s.Detail(c.Request.Context(), User(c), id)
		Reply(c, 200, d, e)
	})
	a.GET("/shipments/:id/events", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		p, l, ok := Page(c)
		if !ok {
			return
		}
		d, e := s.Events(c.Request.Context(), User(c), id, p, l)
		Reply(c, 200, d, e)
	})
	a.PATCH("/drivers/me/shipments/:id/status", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		var in shipping.Update
		if !Bind(c, &in) {
			return
		}
		Reply(c, 200, nil, s.Update(c.Request.Context(), User(c), id, c.GetHeader("Idempotency-Key"), in))
	})
}
