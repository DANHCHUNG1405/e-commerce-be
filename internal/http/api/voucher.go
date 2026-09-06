package api

import (
	"github.com/example/e-commerce-be/internal/modules/voucher"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func VoucherRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, s *voucher.Service) {
	v.GET("/admin/vouchers", auth, func(c *gin.Context) {
		p, l, ok := Page(c)
		if !ok {
			return
		}
		d, e := s.Managed(c.Request.Context(), User(c), nil, p, l)
		Reply(c, 200, d, e)
	})
	v.GET("/sellers/:seller/vouchers/manage", auth, func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		p, l, ok := Page(c)
		if !ok {
			return
		}
		d, e := s.Managed(c.Request.Context(), User(c), &id, p, l)
		Reply(c, 200, d, e)
	})
	v.POST("/vouchers/preview", auth, func(c *gin.Context) {
		var in struct {
			Code       string      `json:"code" binding:"required,max=40"`
			VariantIDs []uuid.UUID `json:"variantIds" binding:"max=100"`
		}
		if !Bind(c, &in) {
			return
		}
		d, e := s.Preview(c.Request.Context(), User(c), in.Code, in.VariantIDs)
		Reply(c, 200, d, e)
	})
	v.GET("/vouchers", func(c *gin.Context) {
		p, l, ok := Page(c)
		if !ok {
			return
		}
		d, e := s.List(c.Request.Context(), nil, p, l)
		Reply(c, 200, d, e)
	})
	v.GET("/sellers/:seller/vouchers", func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		p, l, ok := Page(c)
		if !ok {
			return
		}
		d, e := s.List(c.Request.Context(), &id, p, l)
		Reply(c, 200, d, e)
	})
	create := func(shop bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			var seller *uuid.UUID
			if shop {
				id, ok := ID(c, "seller")
				if !ok {
					return
				}
				seller = &id
			}
			var in voucher.Input
			if !Bind(c, &in) {
				return
			}
			d, e := s.Create(c.Request.Context(), User(c), seller, in)
			Reply(c, 201, d, e)
		}
	}
	v.POST("/admin/vouchers", auth, create(false))
	v.POST("/sellers/:seller/vouchers", auth, create(true))
	v.PATCH("/vouchers/:id/status", auth, func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		var in struct {
			Active *bool `json:"active" binding:"required"`
		}
		if !Bind(c, &in) {
			return
		}
		Reply(c, 200, nil, s.SetActive(c.Request.Context(), User(c), id, *in.Active))
	})
}
