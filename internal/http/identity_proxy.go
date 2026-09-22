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

func identityProxyRoutes(v *gin.RouterGroup, rawURL string) {
	handler := unavailableIdentityProxy
	if target, err := url.Parse(rawURL); err == nil && target.Scheme != "" && target.Host != "" {
		proxy := httputil.NewSingleHostReverseProxy(target)
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext
		transport.ResponseHeaderTimeout = 30 * time.Second
		proxy.Transport = transport
		originalDirector := proxy.Director
		proxy.Director = func(request *http.Request) {
			originalDirector(request)
			request.Host = target.Host
		}
		proxy.ErrorHandler = func(writer http.ResponseWriter, _ *http.Request, _ error) {
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(writer).Encode(response.NewEnvelope(http.StatusServiceUnavailable, "identity service unavailable", nil))
		}
		handler = func(c *gin.Context) {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 35*time.Second)
			defer cancel()
			proxy.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
		}
	}

	v.POST("/auth/register", handler)
	v.POST("/auth/login", handler)
	v.POST("/auth/refresh", handler)
	v.POST("/auth/logout", handler)
	v.POST("/auth/change-password", handler)
	v.POST("/auth/forgot-password", handler)
	v.POST("/auth/reset-password", handler)
	v.GET("/auth/me", handler)
	v.GET("/users/me/permissions", handler)
	v.GET("/users/me/seller-memberships", handler)
	v.PATCH("/users/me", handler)
	v.GET("/users/me/addresses", handler)
	v.POST("/users/me/addresses", handler)
	v.PUT("/users/me/addresses/:id", handler)
	v.DELETE("/users/me/addresses/:id", handler)
}

func unavailableIdentityProxy(c *gin.Context) {
	response.Failure(c, http.StatusServiceUnavailable, "identity service unavailable")
}
