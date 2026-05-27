// Package auth implements user registration, login, and signed session tokens
// for SolderDB. Users are stored in a normal SolderDB collection (`_users`),
// so they ride on top of the same engine and benefit from snapshots/compaction
// like any other data.
//
// Tokens are hand-rolled HMAC-SHA256 signed payloads — no JWT library needed.
// Format: base64url(payload-json).base64url(hmac-sha256(secret, payload-json))
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"solderdb/internal/collections"
	"solderdb/internal/engine"
)

const (
	UsersCollection = "_users"
	SecretFile      = ".secret"

	TokenLifetime = 7 * 24 * time.Hour
	BcryptCost    = 12
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// User is the public representation — never includes the password hash.
type User struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Role    Role   `json:"role"`
	Created string `json:"created"`
	Updated string `json:"updated"`
}

type Session struct {
	Token   string `json:"token"`
	User    User   `json:"user"`
	Expires string `json:"expires"`
}

// tokenPayload is what gets signed.
type tokenPayload struct {
	Sub string `json:"sub"` // user ID
	Iat int64  `json:"iat"` // issued at (unix seconds)
	Exp int64  `json:"exp"` // expires at
}

type Service struct {
	db     *engine.DB
	colls  *collections.Service
	secret []byte
}

func New(db *engine.DB, colls *collections.Service, dataDir string) (*Service, error) {
	secret, err := loadOrCreateSecret(dataDir)
	if err != nil {
		return nil, err
	}
	svc := &Service{db: db, colls: colls, secret: secret}
	if err := svc.ensureUsersCollection(); err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *Service) ensureUsersCollection() error {
	if _, err := s.colls.GetCollection(UsersCollection); err == nil {
		return nil
	}
	_, err := s.colls.CreateCollection(collections.CollectionMeta{
		Name: UsersCollection,
		Fields: []collections.Field{
			{Name: "email", Type: collections.FieldText, Required: true, Unique: true},
			{Name: "password_hash", Type: collections.FieldText, Required: true},
			{Name: "role", Type: collections.FieldText, Required: true},
		},
	})
	return err
}

// ---------------- Public API ----------------

func (s *Service) Register(email, password string) (Session, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !validEmail(email) {
		return Session{}, errors.New("invalid email")
	}
	if len(password) < 8 {
		return Session{}, errors.New("password must be at least 8 characters")
	}
	// Cheap uniqueness check — full unique-constraint indexing is future work.
	if u, err := s.findByEmail(email); err == nil && u != nil {
		return Session{}, errors.New("email already registered")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return Session{}, fmt.Errorf("hash password: %w", err)
	}
	// First registered user becomes admin. Everyone else is a regular user.
	role := RoleUser
	if count, _ := s.userCount(); count == 0 {
		role = RoleAdmin
	}

	rec, err := s.colls.Insert(UsersCollection, map[string]interface{}{
		"email":         email,
		"password_hash": string(hash),
		"role":          string(role),
	})
	if err != nil {
		return Session{}, err
	}
	user := recToUser(rec)
	return s.issueSession(user)
}

func (s *Service) Login(email, password string) (Session, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	rec, err := s.findByEmail(email)
	if err != nil {
		return Session{}, err
	}
	if rec == nil {
		return Session{}, errors.New("invalid credentials")
	}
	hash, _ := rec.Data["password_hash"].(string)
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return Session{}, errors.New("invalid credentials")
	}
	return s.issueSession(recToUser(*rec))
}

// VerifyToken parses + validates a token and returns the user. Use this
// from middleware before serving protected routes.
func (s *Service) VerifyToken(token string) (User, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return User{}, errors.New("malformed token")
	}
	payloadB, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return User{}, errors.New("invalid token encoding")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return User{}, errors.New("invalid token signature")
	}
	expectedSig := s.sign(payloadB)
	if !hmac.Equal(sig, expectedSig) {
		return User{}, errors.New("invalid token signature")
	}
	var p tokenPayload
	if err := json.Unmarshal(payloadB, &p); err != nil {
		return User{}, errors.New("invalid token payload")
	}
	if time.Now().Unix() > p.Exp {
		return User{}, errors.New("token expired")
	}
	rec, err := s.colls.GetRecord(UsersCollection, p.Sub)
	if err != nil {
		return User{}, errors.New("user not found")
	}
	return recToUser(rec), nil
}

// ---------------- Internals ----------------

func (s *Service) issueSession(user User) (Session, error) {
	now := time.Now()
	exp := now.Add(TokenLifetime)
	payload := tokenPayload{Sub: user.ID, Iat: now.Unix(), Exp: exp.Unix()}
	payloadB, err := json.Marshal(payload)
	if err != nil {
		return Session{}, fmt.Errorf("encode payload: %w", err)
	}
	sig := s.sign(payloadB)
	token := base64.RawURLEncoding.EncodeToString(payloadB) + "." + base64.RawURLEncoding.EncodeToString(sig)
	return Session{Token: token, User: user, Expires: exp.UTC().Format(time.RFC3339)}, nil
}

func (s *Service) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func (s *Service) userCount() (int, error) {
	res, err := s.colls.ListRecords(UsersCollection, "", 1)
	if err != nil {
		return 0, err
	}
	if len(res.Records) == 0 {
		return 0, nil
	}
	// Cheap upper-bound check: list many.
	all, err := s.colls.ListRecords(UsersCollection, "", 1000)
	if err != nil {
		return 0, err
	}
	return len(all.Records), nil
}

func (s *Service) findByEmail(email string) (*collections.Record, error) {
	// Linear scan — fine for v1; secondary indexes are a future engine feature.
	res, err := s.colls.ListRecords(UsersCollection, "", 10000)
	if err != nil {
		return nil, err
	}
	for _, r := range res.Records {
		if got, _ := r.Data["email"].(string); strings.EqualFold(got, email) {
			rec := r
			return &rec, nil
		}
	}
	return nil, nil
}

func recToUser(r collections.Record) User {
	role, _ := r.Data["role"].(string)
	email, _ := r.Data["email"].(string)
	return User{
		ID:      r.ID,
		Email:   email,
		Role:    Role(role),
		Created: r.Created,
		Updated: r.Updated,
	}
}

func validEmail(s string) bool {
	if len(s) < 3 || len(s) > 254 {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at < 1 || at == len(s)-1 {
		return false
	}
	if strings.IndexByte(s[at+1:], '.') < 0 {
		return false
	}
	return true
}

func loadOrCreateSecret(dataDir string) ([]byte, error) {
	path := filepath.Join(dataDir, SecretFile)
	if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
		return b, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, fmt.Errorf("write secret: %w", err)
	}
	return b, nil
}
