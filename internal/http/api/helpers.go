package api

import (
	"errors"
	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"net/http"
	"strconv"
)

func Reply(c *gin.Context, status int, data any, err error) {
	if err == nil {
		response.Success(c, status, data)
		return
	}
	code := 500
	message := "internal server error"
	var pg *pgconn.PgError
	switch {
	case errors.Is(err, shared.ErrRateLimited):
		code = 429
		message = "rate limit exceeded"
	case errors.Is(err, shared.ErrUnavailable):
		code = 503
		message = "service unavailable"
	case errors.Is(err, shared.ErrForbidden):
		code = 403
		message = "forbidden"
	case errors.Is(err, gorm.ErrRecordNotFound):
		code = 404
		message = "not found"
	case errors.Is(err, shared.ErrInvalid):
		code = 400
		message = "invalid input"
	case errors.Is(err, shared.ErrConflict):
		code = 409
		message = "conflict"
	case errors.As(err, &pg):
		if pg.Code == "23505" {
			code = 409
			message = "already exists"
		} else if pg.Code == "23503" || pg.Code == "23514" {
			code = 400
			message = "invalid reference or value"
		}
	}
	response.Failure(c, code, message)
}
func User(c *gin.Context) uuid.UUID { id, _ := uuid.Parse(c.GetString("user_id")); return id }
func ID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil || id == uuid.Nil {
		response.Abort(c, 400, "invalid UUID")
		return uuid.Nil, false
	}
	return id, true
}
func Bind(c *gin.Context, dst any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	if c.ShouldBindJSON(dst) != nil {
		response.Failure(c, 400, "invalid request")
		return false
	}
	return true
}
func Page(c *gin.Context) (int, int, bool) {
	p, e := strconv.Atoi(c.DefaultQuery("page", "1"))
	l, f := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if e != nil || f != nil || p < 1 || p > 100000 || l < 1 || l > 100 {
		response.Failure(c, 400, "page must be 1..100000 and limit 1..100")
		return 0, 0, false
	}
	return p, l, true
}
