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

func (h *Handler) ChangePassword(c *gin.Context) {
	var input struct {
		CurrentPassword string `json:"currentPassword" binding:"required"`
		NewPassword     string `json:"newPassword" binding:"required,min=8"`
	}
	if c.ShouldBindJSON(&input) != nil {
		response.Failure(c, http.StatusBadRequest, "invalid request")
		return
	}
	id, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		response.Failure(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	err = h.service.ChangePassword(c.Request.Context(), id, input.CurrentPassword, input.NewPassword)
	if errors.Is(err, ErrInvalidCredentials) {
		response.Failure(c, http.StatusBadRequest, "current password is invalid")
		return
	}
	if err != nil {
		response.Failure(c, http.StatusInternalServerError, "could not change password")
		return
	}
	response.Success(c, http.StatusOK, nil)
}

func (h *Handler) ForgotPassword(c *gin.Context) {
	var input struct {
		Email string `json:"email" binding:"required,email"`
	}
	if c.ShouldBindJSON(&input) != nil {
		response.Failure(c, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.service.ForgotPassword(c.Request.Context(), input.Email); err != nil {
		if errors.Is(err, ErrEmailUnavailable) {
			response.Failure(c, http.StatusServiceUnavailable, "email service unavailable")
			return
		}
		response.Failure(c, http.StatusInternalServerError, "could not process password reset")
		return
	}
	response.Success(c, http.StatusOK, gin.H{"message": "if the email exists, a reset link has been sent"})
}

func (h *Handler) ResetPassword(c *gin.Context) {
	var input struct {
		Token       string `json:"token" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required,min=8"`
	}
	if c.ShouldBindJSON(&input) != nil {
		response.Failure(c, http.StatusBadRequest, "invalid request")
		return
	}
	if err := h.service.ResetPassword(c.Request.Context(), input.Token, input.NewPassword); err != nil {
		if errors.Is(err, ErrInvalidToken) {
			response.Failure(c, http.StatusBadRequest, "invalid or expired reset token")
			return
		}
		response.Failure(c, http.StatusInternalServerError, "could not reset password")
		return
	}
	response.Success(c, http.StatusOK, nil)
}
