package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	coreauth "github.com/example/e-commerce-be/internal/auth"
	"github.com/example/e-commerce-be/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailExists        = errors.New("email already exists")
	ErrInvalidToken       = errors.New("invalid token")
)

type Service struct {
	db     *gorm.DB
	tokens coreauth.TokenService
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

func NewService(db *gorm.DB, tokens coreauth.TokenService) *Service {
	return &Service{db: db, tokens: tokens}
}

func (s *Service) Register(input RegisterInput) (*models.User, TokenPair, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if email == "" || len(input.Password) < 8 {
		return nil, TokenPair{}, ErrInvalidCredentials
	}
	hash, err := coreauth.HashPassword(input.Password)
	if err != nil {
		return nil, TokenPair{}, err
	}
	user := &models.User{Email: email, Password: hash, FullName: strings.TrimSpace(input.FullName)}
	err = s.db.Transaction(func(tx *gorm.DB) error {
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
	pair, err := s.issueTokens(user.ID, "customer")
	return user, pair, err
}

func (s *Service) Login(email, password string) (*models.User, TokenPair, error) {
	var user models.User
	if err := s.db.Where("email = ?", strings.ToLower(strings.TrimSpace(email))).First(&user).Error; err != nil {
		return nil, TokenPair{}, ErrInvalidCredentials
	}
	if coreauth.VerifyPassword(user.Password, password) != nil {
		return nil, TokenPair{}, ErrInvalidCredentials
	}
	role := "customer"
	_ = s.db.Table("roles").Select("roles.name").Joins("JOIN user_roles ON user_roles.role_id = roles.id").Where("user_roles.user_id = ?", user.ID).Scan(&role).Error
	pair, err := s.issueTokens(user.ID, role)
	return &user, pair, err
}

func (s *Service) Refresh(token string) (TokenPair, error) {
	claims, err := s.tokens.Parse(token)
	if err != nil || claims.Type != "refresh" {
		return TokenPair{}, ErrInvalidToken
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return TokenPair{}, ErrInvalidToken
	}
	hash := hashToken(token)
	var stored models.RefreshToken
	if err := s.db.Where("user_id = ? AND token_hash = ? AND revoked_at IS NULL AND expires_at > ?", userID, hash, time.Now()).First(&stored).Error; err != nil {
		return TokenPair{}, ErrInvalidToken
	}
	now := time.Now()
	result := s.db.Model(&models.RefreshToken{}).Where("id = ? AND revoked_at IS NULL", stored.ID).Updates(map[string]any{"revoked_at": now})
	if result.Error != nil {
		return TokenPair{}, result.Error
	}
	if result.RowsAffected != 1 {
		return TokenPair{}, ErrInvalidToken
	}
	return s.issueTokens(userID, claims.Role)
}

func (s *Service) Logout(token string) error {
	claims, err := s.tokens.Parse(token)
	if err != nil || claims.Type != "refresh" {
		return ErrInvalidToken
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return ErrInvalidToken
	}
	now := time.Now()
	return s.db.Model(&models.RefreshToken{}).Where("user_id = ? AND token_hash = ? AND revoked_at IS NULL", userID, hashToken(token)).Updates(map[string]any{"revoked_at": now}).Error
}

func (s *Service) FindUser(id uuid.UUID) (*models.User, error) {
	var user models.User
	if err := s.db.First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) issueTokens(userID uuid.UUID, role string) (TokenPair, error) {
	access, err := s.tokens.Generate(userID, role, 15*time.Minute)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := s.tokens.GenerateRefresh(userID, role, 30*24*time.Hour)
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.db.Create(&models.RefreshToken{UserID: userID, TokenHash: hashToken(refresh), ExpiresAt: time.Now().Add(30 * 24 * time.Hour)}).Error; err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: refresh, ExpiresIn: 900}, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
