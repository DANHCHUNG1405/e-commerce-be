package auth

import (
	"errors"
	"net/http"

	"github.com/example/e-commerce-be/internal/http/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Register(c *gin.Context) {
	var input RegisterInput
	if c.ShouldBindJSON(&input) != nil {
		response.Failure(c, http.StatusBadRequest, "invalid request")
		return
	}
	user, tokens, err := h.service.Register(c.Request.Context(), input)
	if errors.Is(err, ErrEmailExists) {
		response.Failure(c, http.StatusConflict, "email already exists")
		return
	}
	if errors.Is(err, ErrInvalidCredentials) {
		response.Failure(c, http.StatusBadRequest, "email and password are invalid")
		return
	}
	if err != nil {
		response.Failure(c, http.StatusInternalServerError, "could not create account")
		return
	}
	response.Success(c, http.StatusCreated, gin.H{"user": user, "tokens": tokens})
}

func (h *Handler) Login(c *gin.Context) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&input) != nil {
		response.Failure(c, http.StatusBadRequest, "invalid request")
		return
	}
	user, tokens, err := h.service.Login(c.Request.Context(), input.Email, input.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		response.Failure(c, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		response.Failure(c, http.StatusInternalServerError, "could not sign in")
		return
	}
	response.Success(c, http.StatusOK, gin.H{"user": user, "tokens": tokens})
}

func (h *Handler) Refresh(c *gin.Context) {
	var input struct {
		RefreshToken string `json:"refreshToken"`
	}
	if c.ShouldBindJSON(&input) != nil || input.RefreshToken == "" {
		response.Failure(c, http.StatusBadRequest, "refreshToken is required")
		return
	}
	tokens, err := h.service.Refresh(c.Request.Context(), input.RefreshToken)
	if errors.Is(err, ErrInvalidToken) {
		response.Failure(c, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	if err != nil {
		response.Failure(c, http.StatusInternalServerError, "could not refresh token")
		return
	}
	response.Success(c, http.StatusOK, tokens)
}

func (h *Handler) Logout(c *gin.Context) {
	var input struct {
		RefreshToken string `json:"refreshToken"`
	}
	if c.ShouldBindJSON(&input) != nil || input.RefreshToken == "" {
		response.Failure(c, http.StatusBadRequest, "refreshToken is required")
		return
	}
	err := h.service.Logout(c.Request.Context(), input.RefreshToken)
	if errors.Is(err, ErrInvalidToken) {
		response.Failure(c, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	if err != nil {
		response.Failure(c, http.StatusInternalServerError, "could not sign out")
		return
	}
	response.Success(c, http.StatusOK, nil)
}

func (h *Handler) Me(c *gin.Context) {
	id, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		response.Failure(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := h.service.FindUser(c.Request.Context(), id)
	if err != nil {
		response.Failure(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	response.Success(c, http.StatusOK, user)
}
