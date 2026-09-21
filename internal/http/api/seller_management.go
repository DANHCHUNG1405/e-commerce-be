package api

import (
	"github.com/example/e-commerce-be/internal/modules/seller"
	"github.com/gin-gonic/gin"
)

func SellerManagementRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, s *seller.Management) {
	v.GET("/admin/sellers", auth, func(c *gin.Context) {
		p, l, ok := Page(c)
		if !ok {
			return
		}
		items, err := s.AdminList(c.Request.Context(), User(c), c.Query("status"), p, l)
		Reply(c, 200, items, err)
	})
	a := v.Group("/sellers/:seller", auth)
	a.GET("/profile", func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		d, e := s.Profile(c.Request.Context(), User(c), id)
		Reply(c, 200, d, e)
	})
	a.PUT("/profile", func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		var in seller.ShopInput
		if !Bind(c, &in) {
			return
		}
		Reply(c, 200, nil, s.Update(c.Request.Context(), User(c), id, in))
	})
	for _, asset := range []string{"logo", "banner"} {
		name := asset
		a.POST("/"+name, func(c *gin.Context) {
			var in seller.SellerAssetInput
			if !Bind(c, &in) {
				return
			}
			id, ok := ID(c, "seller")
			if !ok {
				return
			}
			Reply(c, 200, nil, s.SetAsset(c.Request.Context(), User(c), id, name+"_url", in.URL))
		})
		a.DELETE("/"+name, func(c *gin.Context) {
			id, ok := ID(c, "seller")
			if !ok {
				return
			}
			Reply(c, 200, nil, s.SetAsset(c.Request.Context(), User(c), id, name+"_url", ""))
		})
	}
	a.GET("/members", func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		p, l, ok := Page(c)
		if !ok {
			return
		}
		d, e := s.Members(c.Request.Context(), User(c), id, p, l)
		Reply(c, 200, d, e)
	})
	member := func(remove bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			id, ok := ID(c, "seller")
			if !ok {
				return
			}
			target, ok := ID(c, "user")
			if !ok {
				return
			}
			var in struct {
				Role string `json:"role" binding:"required,oneof=manager staff"`
			}
			if !remove && !Bind(c, &in) {
				return
			}
			Reply(c, 200, nil, s.SetMember(c.Request.Context(), User(c), id, target, in.Role))
		}
	}
	a.PUT("/members/:user", member(false))
	a.DELETE("/members/:user", member(true))
	a.GET("/dashboard", func(c *gin.Context) {
		id, ok := ID(c, "seller")
		if !ok {
			return
		}
		d, e := s.Dashboard(c.Request.Context(), User(c), id)
		Reply(c, 200, d, e)
	})
	a.GET("/orders/:id", func(c *gin.Context) {
		shop, ok := ID(c, "seller")
		if !ok {
			return
		}
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		d, e := s.Detail(c.Request.Context(), User(c), shop, id)
		Reply(c, 200, d, e)
	})
	a.GET("/variants/:variant/inventory", func(c *gin.Context) {
		shop, ok := ID(c, "seller")
		if !ok {
			return
		}
		id, ok := ID(c, "variant")
		if !ok {
			return
		}
		p, l, ok := Page(c)
		if !ok {
			return
		}
		d, e := s.Inventory(c.Request.Context(), User(c), shop, id, p, l)
		Reply(c, 200, d, e)
	})
}
