package httpserver

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/gin-gonic/gin"
)

func sellerProxyRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, rawURL string) {
	handler := func(c *gin.Context) { response.Failure(c, http.StatusServiceUnavailable, "seller service unavailable") }
	if target, err := url.Parse(rawURL); err == nil && (target.Scheme == "http" || target.Scheme == "https") && target.Host != "" {
		proxy := httputil.NewSingleHostReverseProxy(target)
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext
		transport.ResponseHeaderTimeout = 5 * time.Second
		proxy.Transport = transport
		originalDirector := proxy.Director
		proxy.Director = func(request *http.Request) {
			originalDirector(request)
			request.Host = target.Host
		}
		proxy.ErrorHandler = func(writer http.ResponseWriter, _ *http.Request, _ error) {
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(writer).Encode(response.NewEnvelope(http.StatusServiceUnavailable, "seller service unavailable", nil))
		}
		handler = func(c *gin.Context) {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 6*time.Second)
			defer cancel()
			proxy.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
		}
	}
	v.GET("/sellers/:seller", handler)
	a := v.Group("", auth)
	a.POST("/sellers", handler)
	a.GET("/users/me/sellers", handler)
	a.GET("/admin/sellers", handler)
	a.PATCH("/admin/sellers/:seller/status", handler)
	a.GET("/sellers/:seller/profile", handler)
	a.PUT("/sellers/:seller/profile", handler)
	for _, asset := range []string{"logo", "banner"} {
		a.POST("/sellers/:seller/"+asset, handler)
		a.DELETE("/sellers/:seller/"+asset, handler)
	}
	a.GET("/sellers/:seller/members", handler)
	a.PUT("/sellers/:seller/members/:user", handler)
	a.DELETE("/sellers/:seller/members/:user", handler)
}
