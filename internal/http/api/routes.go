package api

import (
	"github.com/example/e-commerce-be/internal/modules/cart"
	"github.com/example/e-commerce-be/internal/modules/catalog"
	"github.com/example/e-commerce-be/internal/modules/order"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func Register(v *gin.RouterGroup, auth gin.HandlerFunc, cat *catalog.Service, carts *cart.Service, orders *order.Service) {
	v.GET("/products", func(c *gin.Context) {
		p, l, ok := Page(c)
		if !ok {
			return
		}
		var filter catalog.Filter
		if c.ShouldBindQuery(&filter) != nil {
			Reply(c, 400, nil, shared.ErrInvalid)
			return
		}
		for name, dst := range map[string]**uuid.UUID{"sellerId": &filter.SellerID, "categoryId": &filter.CategoryID} {
			if raw, exists := c.GetQuery(name); exists {
				id, err := uuid.Parse(raw)
				if err != nil || id == uuid.Nil {
					Reply(c, 400, nil, shared.ErrInvalid)
					return
				}
				*dst = &id
			}
		}
		data, err := cat.Search(c.Request.Context(), filter, p, l)
		Reply(c, 200, data, err)
	})
	v.GET("/products/:id", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		data, err := cat.Detail(c.Request.Context(), id)
		Reply(c, 200, data, err)
	})
	v.GET("/categories", func(c *gin.Context) {
		p, l, ok := Page(c)
		if !ok {
			return
		}
		data, err := cat.Categories(c.Request.Context(), p, l)
		Reply(c, 200, data, err)
	})
	a := v.Group("", auth)
	a.POST("/admin/categories", func(c *gin.Context) {
		var in struct {
			Name     string     `json:"name" binding:"required,max=200"`
			Slug     string     `json:"slug" binding:"required,max=200"`
			ParentID *uuid.UUID `json:"parentId"`
		}
		if !Bind(c, &in) {
			return
		}
		data, err := cat.Category(c.Request.Context(), User(c), in.Name, in.Slug, in.ParentID)
		Reply(c, 201, data, err)
	})
	a.POST("/sellers/:seller/products", func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		var in catalog.ProductInput
		if !Bind(c, &in) {
			return
		}
		data, err := cat.Save(c.Request.Context(), User(c), id, uuid.Nil, in)
		Reply(c, 201, data, err)
	})
	a.PUT("/sellers/:seller/products/:product", func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		pid, ok := ID(c, "product")
		if !ok {
			return
		}
		var in catalog.ProductInput
		if !Bind(c, &in) {
			return
		}
		data, err := cat.Save(c.Request.Context(), User(c), id, pid, in)
		Reply(c, 200, data, err)
	})
	a.POST("/sellers/:seller/products/:product/variants", func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		pid, ok := ID(c, "product")
		if !ok {
			return
		}
		var in catalog.VariantInput
		if !Bind(c, &in) {
			return
		}
		data, err := cat.Variant(c.Request.Context(), User(c), id, pid, in)
		Reply(c, 201, data, err)
	})
	a.POST("/sellers/:seller/variants/:variant/inventory", func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		vid, ok := ID(c, "variant")
		if !ok {
			return
		}
		var in struct {
			Delta int    `json:"delta" binding:"required"`
			Note  string `json:"note" binding:"required,max=500"`
		}
		if !Bind(c, &in) {
			return
		}
		Reply(c, 200, nil, cat.Adjust(c.Request.Context(), User(c), id, vid, in.Delta, in.Note))
	})
	a.GET("/cart", func(c *gin.Context) { data, err := carts.Items(c.Request.Context(), User(c)); Reply(c, 200, data, err) })
	a.PUT("/cart/items/:variant", func(c *gin.Context) {
		id, ok := ID(c, "variant")
		if !ok {
			return
		}
		var in struct {
			Quantity int `json:"quantity" binding:"required,gte=1,lte=10000"`
		}
		if !Bind(c, &in) {
			return
		}
		Reply(c, 200, nil, carts.Put(c.Request.Context(), User(c), id, in.Quantity))
	})
	a.DELETE("/cart/items/:variant", func(c *gin.Context) {
		id, ok := ID(c, "variant")
		if !ok {
			return
		}
		Reply(c, 200, nil, carts.Put(c.Request.Context(), User(c), id, 0))
	})
	a.POST("/orders", func(c *gin.Context) {
		var in struct {
			AddressID  uuid.UUID   `json:"addressId" binding:"required"`
			Method     string      `json:"method" binding:"required,oneof=cod sepay"`
			VariantIDs []uuid.UUID `json:"variantIds" binding:"max=100"`
			CouponCode string      `json:"couponCode" binding:"max=40"`
		}
		if !Bind(c, &in) {
			return
		}
		data, err := orders.CheckoutVoucher(c.Request.Context(), User(c), in.AddressID, c.GetHeader("Idempotency-Key"), in.Method, in.VariantIDs, in.CouponCode)
		Reply(c, 201, data, err)
	})
	a.GET("/orders", func(c *gin.Context) {
		p, l, ok := Page(c)
		if !ok {
			return
		}
		data, err := orders.List(c.Request.Context(), User(c), p, l)
		Reply(c, 200, data, err)
	})
	a.GET("/orders/:id", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		data, err := orders.Detail(c.Request.Context(), User(c), id)
		Reply(c, 200, data, err)
	})
	a.POST("/orders/:id/cancel", func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		Reply(c, 200, nil, orders.Cancel(c.Request.Context(), User(c), id))
	})
}
