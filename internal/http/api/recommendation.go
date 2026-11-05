package api

import (
	"net/http"
	"strconv"

	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/example/e-commerce-be/internal/modules/recommendation"
	"github.com/gin-gonic/gin"
)

func RecommendationRoutes(v *gin.RouterGroup, auth gin.HandlerFunc, service *recommendation.Service) {
	v.GET("/recommendations/home", auth, func(c *gin.Context) {
		limit, ok := recommendationLimit(c)
		if !ok {
			return
		}
		result, err := service.Home(c.Request.Context(), User(c), limit)
		Reply(c, http.StatusOK, result, err)
	})
	v.GET("/products/:id/recommendations", auth, func(c *gin.Context) {
		id, ok := ID(c, "id")
		if !ok {
			return
		}
		limit, ok := recommendationLimit(c)
		if !ok {
			return
		}
		result, err := service.Similar(c.Request.Context(), User(c), id, limit)
		Reply(c, http.StatusOK, result, err)
	})
}

func recommendationLimit(c *gin.Context) (int, bool) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "12"))
	if err != nil || limit < 1 || limit > 20 {
		response.Failure(c, 400, "limit must be 1..20")
		return 0, false
	}
	return limit, true
}
