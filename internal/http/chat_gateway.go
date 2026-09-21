package httpserver

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/http/api"
	"github.com/example/e-commerce-be/internal/http/middleware"
	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/gin-gonic/gin"
)

func AttachChatGateway(router *gin.Engine, tokens coreauth.TokenService, service api.ChatService, rawHTTPURL string) {
	api.ChatRoutes(router.Group("/api/v1"), middleware.RequireAccessToken(tokens), service)

	handler := unavailableChatProxy
	if target, err := url.Parse(rawHTTPURL); err == nil && target.Scheme != "" && target.Host != "" {
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
			_ = json.NewEncoder(writer).Encode(response.NewEnvelope(http.StatusServiceUnavailable, "chat service unavailable", nil))
		}
		handler = func(c *gin.Context) { proxy.ServeHTTP(c.Writer, c.Request) }
	}
	router.GET("/api/v1/ws", handler)
}

func unavailableChatProxy(c *gin.Context) {
	response.Failure(c, http.StatusServiceUnavailable, "chat service unavailable")
}
