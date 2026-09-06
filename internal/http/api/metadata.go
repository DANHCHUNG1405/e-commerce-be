package api

import (
	"github.com/example/e-commerce-be/internal/modules/catalog"
	"github.com/gin-gonic/gin"
)

func MetadataRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, s *catalog.Service, r *catalog.MetadataRepository) {
	v.GET("/products/:id/metadata", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		d, e := s.GetMetadata(c.Request.Context(), r, id)
		Reply(c, 200, d, e)
	})
	v.PUT("/sellers/:seller/products/:product/metadata", auth, func(c *gin.Context) {
		seller, ok := ID(c, "seller")
		if !ok {
			return
		}
		id, ok := ID(c, "product")
		if !ok {
			return
		}
		var in catalog.Metadata
		if !Bind(c, &in) {
			return
		}
		Reply(c, 200, nil, s.PutMetadata(c.Request.Context(), r, User(c), seller, id, in))
	})
}
