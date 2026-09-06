package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailExists        = errors.New("email already exists")
	ErrInvalidToken       = errors.New("invalid token")
)

type Service struct {
	db            *gorm.DB
	tokens        coreauth.TokenService
	refreshTokens *refreshTokenStore
}
type RegisterInput struct {
	Email    string
	Password string
	FullName string
}
type TokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int64  `json:"expiresIn"`
}

func NewService(db *gorm.DB, tokens coreauth.TokenService, redisClient *redis.Client) *Service {
	return &Service{db: db, tokens: tokens, refreshTokens: newRefreshTokenStore(redisClient)}
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (*models.User, TokenPair, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if email == "" || len(input.Password) < 8 {
		return nil, TokenPair{}, ErrInvalidCredentials
	}
	hash, err := coreauth.HashPassword(input.Password)
	if err != nil {
		return nil, TokenPair{}, err
	}
	user := &models.User{Email: email, Password: hash, FullName: strings.TrimSpace(input.FullName)}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("email = ?", email).First(&models.User{}).Error; err == nil {
			return ErrEmailExists
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		var role models.Role
		if err := tx.Where("name = ?", "customer").First(&role).Error; err != nil {
			return err
		}
		return tx.Create(&models.UserRole{UserID: user.ID, RoleID: role.ID}).Error
	})
	if err != nil {
		return nil, TokenPair{}, err
	}
	pair, err := s.issueTokens(ctx, user.ID, "customer")
	return user, pair, err
}

func (s *Service) Login(ctx context.Context, email, password string) (*models.User, TokenPair, error) {
	var user models.User
	db := s.db.WithContext(ctx)
	if err := db.Where("email = ?", strings.ToLower(strings.TrimSpace(email))).First(&user).Error; err != nil {
		return nil, TokenPair{}, ErrInvalidCredentials
	}
	if coreauth.VerifyPassword(user.Password, password) != nil {
		return nil, TokenPair{}, ErrInvalidCredentials
	}
	role := "customer"
	_ = db.Table("roles").Select("roles.name").Joins("JOIN user_roles ON user_roles.role_id = roles.id").Where("user_roles.user_id = ?", user.ID).Scan(&role).Error
	pair, err := s.issueTokens(ctx, user.ID, role)
	return &user, pair, err
}

func (s *Service) Refresh(ctx context.Context, token string) (TokenPair, error) {
	claims, err := s.tokens.Parse(token)
	if err != nil || claims.Type != "refresh" {
		return TokenPair{}, ErrInvalidToken
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return TokenPair{}, ErrInvalidToken
	}
	consumed, err := s.refreshTokens.Consume(ctx, hashToken(token), userID)
	if err != nil {
		return TokenPair{}, err
	}
	if !consumed {
		return TokenPair{}, ErrInvalidToken
	}
	return s.issueTokens(ctx, userID, claims.Role)
}

func (s *Service) Logout(ctx context.Context, token string) error {
	claims, err := s.tokens.Parse(token)
	if err != nil || claims.Type != "refresh" {
		return ErrInvalidToken
	}
	if _, err := uuid.Parse(claims.Subject); err != nil {
		return ErrInvalidToken
	}
	deleted, err := s.refreshTokens.Delete(ctx, hashToken(token))
	if err != nil {
		return err
	}
	if !deleted {
		return ErrInvalidToken
	}
	return nil
}

func (s *Service) FindUser(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) issueTokens(ctx context.Context, userID uuid.UUID, role string) (TokenPair, error) {
	access, err := s.tokens.Generate(userID, role, 15*time.Minute)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := s.tokens.GenerateRefresh(userID, role, 30*24*time.Hour)
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.refreshTokens.Store(ctx, hashToken(refresh), userID, 30*24*time.Hour); err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: refresh, ExpiresIn: 900}, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
