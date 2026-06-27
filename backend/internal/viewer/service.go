package viewer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrInvalidUsername    = errors.New("username must contain 3 to 32 printable characters")
	ErrInvalidPassword    = errors.New("password must contain 10 to 128 characters")
	ErrUsernameTaken      = errors.New("username already exists")
)

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	CreatedAt int64  `json:"createdAt"`
}

type Session struct {
	Token     string
	ExpiresAt time.Time
}

type Service struct {
	db         *sql.DB
	sessionTTL time.Duration
	now        func() time.Time
}

func NewService(db *sql.DB, sessionTTL time.Duration) *Service {
	return &Service{db: db, sessionTTL: sessionTTL, now: time.Now}
}

func (s *Service) Register(ctx context.Context, username, password string) (User, Session, error) {
	username = strings.TrimSpace(username)
	if err := validateUsername(username); err != nil {
		return User{}, Session{}, err
	}
	if err := validatePassword(password); err != nil {
		return User{}, Session{}, err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("hash password: %w", err)
	}
	session, tokenHash, err := s.newSession()
	if err != nil {
		return User{}, Session{}, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return User{}, Session{}, err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM viewer_users WHERE username = ?)", username).Scan(&exists); err != nil {
		return User{}, Session{}, err
	}
	if exists {
		return User{}, Session{}, ErrUsernameTaken
	}

	now := s.now().UTC().Unix()
	result, err := tx.ExecContext(ctx,
		"INSERT INTO viewer_users(username, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?)",
		username, string(passwordHash), now, now,
	)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("create viewer user: %w", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		return User{}, Session{}, err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO viewer_sessions(token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		tokenHash[:], userID, now, session.ExpiresAt.Unix(),
	); err != nil {
		return User{}, Session{}, fmt.Errorf("create viewer session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return User{}, Session{}, err
	}
	return User{ID: userID, Username: username, CreatedAt: now}, session, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (User, Session, error) {
	username = strings.TrimSpace(username)
	var user User
	var passwordHash string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, username, password_hash, created_at FROM viewer_users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &passwordHash, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, Session{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return User{}, Session{}, ErrInvalidCredentials
	}

	session, tokenHash, err := s.newSession()
	if err != nil {
		return User{}, Session{}, err
	}
	now := s.now().UTC().Unix()
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO viewer_sessions(token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		tokenHash[:], user.ID, now, session.ExpiresAt.Unix(),
	); err != nil {
		return User{}, Session{}, fmt.Errorf("create viewer session: %w", err)
	}
	return user, session, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrUnauthorized
	}
	tokenHash := sha256.Sum256([]byte(token))
	var user User
	err := s.db.QueryRowContext(ctx, `
SELECT viewer_users.id, viewer_users.username, viewer_users.created_at
FROM viewer_sessions
JOIN viewer_users ON viewer_users.id = viewer_sessions.user_id
WHERE viewer_sessions.token_hash = ? AND viewer_sessions.expires_at > ?`, tokenHash[:], s.now().UTC().Unix()).
		Scan(&user.ID, &user.Username, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	tokenHash := sha256.Sum256([]byte(token))
	_, err := s.db.ExecContext(ctx, "DELETE FROM viewer_sessions WHERE token_hash = ?", tokenHash[:])
	return err
}

func (s *Service) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM viewer_sessions WHERE expires_at <= ?", s.now().UTC().Unix())
	return err
}

func (s *Service) newSession() (Session, [32]byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, [32]byte{}, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expiresAt := s.now().UTC().Add(s.sessionTTL)
	return Session{Token: token, ExpiresAt: expiresAt}, sha256.Sum256([]byte(token)), nil
}

func validateUsername(username string) error {
	length := utf8.RuneCountInString(username)
	if length < 3 || length > 32 {
		return ErrInvalidUsername
	}
	for _, r := range username {
		if unicode.IsControl(r) {
			return ErrInvalidUsername
		}
	}
	return nil
}

func validatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < 10 || length > 128 {
		return ErrInvalidPassword
	}
	return nil
}
