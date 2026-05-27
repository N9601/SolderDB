package bridge

import (
	"fmt"

	"solderdb/internal/auth"
)

// AuthService is exposed to Wails for in-app login/registration.
type AuthService struct {
	svc *auth.Service
}

func NewAuthService(svc *auth.Service) *AuthService {
	return &AuthService{svc: svc}
}

// DTOs mirror the auth package — kept here so Wails generates clean TS classes.

type User struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	Created string `json:"created"`
	Updated string `json:"updated"`
}

type Session struct {
	Token   string `json:"token"`
	User    User   `json:"user"`
	Expires string `json:"expires"`
}

func fromAuthUser(u auth.User) User {
	return User{ID: u.ID, Email: u.Email, Role: string(u.Role), Created: u.Created, Updated: u.Updated}
}

func fromAuthSession(s auth.Session) Session {
	return Session{Token: s.Token, User: fromAuthUser(s.User), Expires: s.Expires}
}

func (a *AuthService) Register(email, password string) (Session, error) {
	if a.svc == nil {
		return Session{}, fmt.Errorf("auth not configured")
	}
	s, err := a.svc.Register(email, password)
	if err != nil {
		return Session{}, err
	}
	return fromAuthSession(s), nil
}

func (a *AuthService) Login(email, password string) (Session, error) {
	if a.svc == nil {
		return Session{}, fmt.Errorf("auth not configured")
	}
	s, err := a.svc.Login(email, password)
	if err != nil {
		return Session{}, err
	}
	return fromAuthSession(s), nil
}

func (a *AuthService) ChangePassword(userID, currentPassword, newPassword string) (User, error) {
	if a.svc == nil {
		return User{}, fmt.Errorf("auth not configured")
	}
	u, err := a.svc.ChangePassword(userID, currentPassword, newPassword)
	if err != nil {
		return User{}, err
	}
	return fromAuthUser(u), nil
}

func (a *AuthService) VerifyToken(token string) (User, error) {
	if a.svc == nil {
		return User{}, fmt.Errorf("auth not configured")
	}
	u, err := a.svc.VerifyToken(token)
	if err != nil {
		return User{}, err
	}
	return fromAuthUser(u), nil
}
