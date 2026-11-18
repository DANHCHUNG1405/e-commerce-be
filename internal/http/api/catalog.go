package api

import (
	"io"
	"net/http"
	"strings"

	"github.com/example/e-commerce-be/internal/modules/catalog"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/gin-gonic/gin"
)

func CatalogRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, s *catalog.Service, imageKitEndpoint ...string) {
	if len(imageKitEndpoint) > 0 {
		s.SetImageKitEndpoint(imageKitEndpoint[0])
	}
	v.POST("/products/search-by-image", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, (10<<20)+(1<<20))
		file, _, err := c.Request.FormFile("image")
		if err != nil {
			Reply(c, http.StatusBadRequest, nil, shared.ErrInvalid)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, (10<<20)+1))
		if err != nil || len(data) == 0 || len(data) > 10<<20 {
			Reply(c, http.StatusBadRequest, nil, shared.ErrInvalid)
			return
		}
		contentType := http.DetectContentType(data)
		if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
			contentType = "image/webp"
		}
		if !strings.HasPrefix(contentType, "image/") || !(strings.Contains(contentType, "jpeg") || strings.Contains(contentType, "png") || strings.Contains(contentType, "gif") || contentType == "image/webp") {
			Reply(c, http.StatusBadRequest, nil, shared.ErrInvalid)
			return
		}
		page, limit, ok := Page(c)
		if !ok {
			return
		}
		products, err := s.SearchByImage(c.Request.Context(), data, contentType, page, limit)
		Reply(c, http.StatusOK, products, err)
	})
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
	v.POST("/sellers/:seller/products/:product/images", auth, func(c *gin.Context) {
		seller, ok := ID(c, "seller")
		if !ok {
			return
		}
		product, ok := ID(c, "product")
		if !ok {
			return
		}
		var in struct {
			URL string `json:"url" binding:"required,max=2048"`
		}
		if !Bind(c, &in) {
			return
		}
		data, err := s.AddImage(c.Request.Context(), User(c), seller, product, in.URL)
		Reply(c, 201, data, err)
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
