package httpserver

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/http/middleware"
	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/gin-gonic/gin"
)

func AttachBehaviorGateway(router *gin.Engine, tokens coreauth.TokenService, rawURL string) {
	endpoint := router.Group("/api/v1/behavior", middleware.RequireAccessToken(tokens))
	target, err := url.Parse(rawURL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		endpoint.POST("/events", func(c *gin.Context) { response.Failure(c, 503, "behavior service unavailable") })
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.ResponseHeaderTimeout = 4 * time.Second
	proxy.Transport = transport
	baseDirector := proxy.Director
	proxy.Director = func(req *http.Request) { baseDirector(req); req.Host = target.Host }
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(503)
		_ = json.NewEncoder(w).Encode(response.NewEnvelope(503, "behavior service unavailable", nil))
	}
	endpoint.POST("/events", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		proxy.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
	})
}
