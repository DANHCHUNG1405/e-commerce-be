package api

import (
	"github.com/example/e-commerce-be/internal/modules/catalog"
	"github.com/gin-gonic/gin"
)

func CatalogRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, s *catalog.Service) {
	v.GET("/sellers/:seller/products/:product", auth, func(c *gin.Context) {
		shop, ok := ID(c, "seller")
		if !ok {
			return
		}
		id, ok := ID(c, "product")
		if !ok {
			return
		}
		d, e := s.SellerDetail(c.Request.Context(), User(c), shop, id)
		Reply(c, 200, d, e)
	})
	v.GET("/sellers/:seller", func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		data, err := s.Shop(c.Request.Context(), id)
		Reply(c, 200, data, err)
	})
	v.GET("/sellers/:seller/products", auth, func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		p, l, ok := Page(c)
		if !ok {
			return
		}
		data, err := s.SellerProducts(c.Request.Context(), User(c), id, p, l, c.Query("status"))
		Reply(c, 200, data, err)
	})
	v.PUT("/sellers/:seller/products/:product/variants/:variant", auth, func(c *gin.Context) {
		seller, ok := ID(c, "seller")
		if !ok {
			return
		}
		product, ok := ID(c, "product")
		if !ok {
			return
		}
		variant, ok := ID(c, "variant")
		if !ok {
			return
		}
		var in catalog.VariantInput
		if !Bind(c, &in) {
			return
		}
		Reply(c, 200, nil, s.UpdateVariant(c.Request.Context(), User(c), seller, product, variant, in))
	})
}
