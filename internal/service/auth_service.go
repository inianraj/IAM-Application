package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"crud/internal/domain"
	"crud/internal/session"
)

// UserLookup finds a user by (lower-cased) email.
type UserLookup interface {
	GetByEmail(ctx context.Context, email string) (domain.User, error)
}

// LoginLimiter counts failed logins per email and IP.
type LoginLimiter interface {
	Blocked(ctx context.Context, email, ip string) (bool, error)
	Fail(ctx context.Context, email, ip string) error
	Reset(ctx context.Context, email, ip string) error
}

// LoginMeta is what a session records about the client that logged in.
type LoginMeta struct {
	IP        string
	UserAgent string
}

type AuthService struct {
	users    UserLookup
	sessions session.Store
	limiter  LoginLimiter
}

func NewAuthService(users UserLookup, sessions session.Store, limiter LoginLimiter) *AuthService {
	return &AuthService{users: users, sessions: sessions, limiter: limiter}
}

// Login signs a user in by email and returns the session token. There is no
// secret to check: knowing the email of an "email" account is enough.
//
// Order of checks: rate limit, user lookup, sign-in method. An unknown email
// counts towards the rate limit, which slows down guessing emails.
func (s *AuthService) Login(ctx context.Context, email string, meta LoginMeta) (string, domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "", domain.User{}, fmt.Errorf("%w: email is required", domain.ErrInvalidUser)
	}

	blocked, err := s.limiter.Blocked(ctx, email, meta.IP)
	if err != nil {
		return "", domain.User{}, fmt.Errorf("%w: %v", domain.ErrUnavailable, err)
	}
	if blocked {
		return "", domain.User{}, domain.ErrTooManyAttempts
	}

	user, err := s.users.GetByEmail(ctx, email)
	if errors.Is(err, domain.ErrUserNotFound) {
		_ = s.limiter.Fail(ctx, email, meta.IP)
		return "", domain.User{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return "", domain.User{}, err
	}

	if err := domain.CheckSignInMethod(user, domain.SignupMethodEmail); err != nil {
		return "", domain.User{}, err
	}

	_ = s.limiter.Reset(ctx, email, meta.IP)

	token, err := s.sessions.Create(ctx, session.Data{
		UserID:         user.ID,
		OrganisationID: user.OrganisationID,
		IP:             meta.IP,
		UserAgent:      meta.UserAgent,
	})
	if err != nil {
		return "", domain.User{}, fmt.Errorf("%w: %v", domain.ErrUnavailable, err)
	}

	return token, user, nil
}

// Logout ends the session. An unknown token is not an error.
func (s *AuthService) Logout(ctx context.Context, token string) error {
	if err := s.sessions.Delete(ctx, token); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrUnavailable, err)
	}
	return nil
}
