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

func notificationProxyRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, rawURL string) {
	handler := unavailableNotificationProxy
	if target, err := url.Parse(rawURL); err == nil && target.Scheme != "" && target.Host != "" {
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
			_ = json.NewEncoder(writer).Encode(response.NewEnvelope(http.StatusServiceUnavailable, "notification service unavailable", nil))
		}
		handler = func(c *gin.Context) {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 6*time.Second)
			defer cancel()
			proxy.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
		}
	}

	a := v.Group("", auth)
	a.GET("/notifications", handler)
	a.GET("/notifications/unread-count", handler)
	a.PATCH("/notifications/:id/read", handler)
	a.PATCH("/notifications/read-all", handler)
}

func unavailableNotificationProxy(c *gin.Context) {
	response.Failure(c, http.StatusServiceUnavailable, "notification service unavailable")
}
