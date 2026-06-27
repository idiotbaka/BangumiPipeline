package viewer

import (
	"bytes"
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
	ErrUserDisabled       = errors.New("viewer user disabled")
	ErrUserNotFound       = errors.New("viewer user not found")
	ErrInvalidSiteName    = errors.New("site name must contain 1 to 80 printable characters")
	ErrInvalidFavicon     = errors.New("favicon must be a png image up to 1 MiB")
)

const DefaultSiteName = "BangumiPipeline Viewer"

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	CreatedAt int64  `json:"createdAt"`
}

type ManagedUser struct {
	ID         int64  `json:"id"`
	Username   string `json:"username"`
	Disabled   bool   `json:"disabled"`
	DisabledAt *int64 `json:"disabledAt"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

type UserPage struct {
	Items    []ManagedUser `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
}

type Session struct {
	Token     string
	ExpiresAt time.Time
}

type SiteSettings struct {
	SiteName         string `json:"siteName"`
	HasFavicon       bool   `json:"hasFavicon"`
	FaviconUpdatedAt *int64 `json:"faviconUpdatedAt"`
	UpdatedAt        int64  `json:"updatedAt"`
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
	var disabledAt sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		"SELECT id, username, password_hash, disabled_at, created_at FROM viewer_users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &passwordHash, &disabledAt, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, Session{}, err
	}
	if disabledAt.Valid {
		return User{}, Session{}, ErrUserDisabled
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
WHERE viewer_sessions.token_hash = ?
  AND viewer_sessions.expires_at > ?
  AND viewer_users.disabled_at IS NULL`, tokenHash[:], s.now().UTC().Unix()).
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

func (s *Service) ListUsers(ctx context.Context, page, pageSize int, query string) (UserPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	result := UserPage{Items: make([]ManagedUser, 0), Page: page, PageSize: pageSize}
	where := "FROM viewer_users"
	args := make([]any, 0, 1)
	if query = strings.TrimSpace(query); query != "" {
		where += " WHERE username LIKE ?"
		args = append(args, "%"+query+"%")
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&result.Total); err != nil {
		return UserPage{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, `
SELECT id, username, disabled_at, created_at, updated_at
`+where+`
ORDER BY created_at DESC, id DESC
LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return UserPage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var user ManagedUser
		var disabledAt sql.NullInt64
		if err := rows.Scan(&user.ID, &user.Username, &disabledAt, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return UserPage{}, err
		}
		if disabledAt.Valid {
			user.Disabled = true
			user.DisabledAt = ptrInt64(disabledAt.Int64)
		}
		result.Items = append(result.Items, user)
	}
	if err := rows.Err(); err != nil {
		return UserPage{}, err
	}
	return result, nil
}

func (s *Service) SetUserDisabled(ctx context.Context, userID int64, disabled bool) (ManagedUser, error) {
	if userID < 1 {
		return ManagedUser{}, ErrUserNotFound
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ManagedUser{}, err
	}
	defer tx.Rollback()

	now := s.now().UTC().Unix()
	var result sql.Result
	if disabled {
		result, err = tx.ExecContext(ctx, `
UPDATE viewer_users
SET disabled_at = COALESCE(disabled_at, ?), updated_at = ?
WHERE id = ?`, now, now, userID)
	} else {
		result, err = tx.ExecContext(ctx, `
UPDATE viewer_users
SET disabled_at = NULL, updated_at = ?
WHERE id = ?`, now, userID)
	}
	if err != nil {
		return ManagedUser{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ManagedUser{}, err
	}
	if affected == 0 {
		return ManagedUser{}, ErrUserNotFound
	}
	if disabled {
		if _, err := tx.ExecContext(ctx, "DELETE FROM viewer_sessions WHERE user_id = ?", userID); err != nil {
			return ManagedUser{}, err
		}
	}
	user, err := scanManagedUser(ctx, tx, userID)
	if err != nil {
		return ManagedUser{}, err
	}
	if err := tx.Commit(); err != nil {
		return ManagedUser{}, err
	}
	return user, nil
}

func (s *Service) ResetUserPassword(ctx context.Context, userID int64, password string) (ManagedUser, error) {
	if userID < 1 {
		return ManagedUser{}, ErrUserNotFound
	}
	if err := validatePassword(password); err != nil {
		return ManagedUser{}, err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return ManagedUser{}, fmt.Errorf("hash password: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ManagedUser{}, err
	}
	defer tx.Rollback()
	now := s.now().UTC().Unix()
	result, err := tx.ExecContext(ctx, `
UPDATE viewer_users
SET password_hash = ?, updated_at = ?
WHERE id = ?`, string(passwordHash), now, userID)
	if err != nil {
		return ManagedUser{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ManagedUser{}, err
	}
	if affected == 0 {
		return ManagedUser{}, ErrUserNotFound
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM viewer_sessions WHERE user_id = ?", userID); err != nil {
		return ManagedUser{}, err
	}
	user, err := scanManagedUser(ctx, tx, userID)
	if err != nil {
		return ManagedUser{}, err
	}
	if err := tx.Commit(); err != nil {
		return ManagedUser{}, err
	}
	return user, nil
}

func (s *Service) SiteSettings(ctx context.Context) (SiteSettings, error) {
	var settings SiteSettings
	var faviconUpdatedAt sql.NullInt64
	var faviconBytes int
	err := s.db.QueryRowContext(ctx, `
SELECT site_name,
       CASE WHEN favicon_png IS NULL THEN 0 ELSE length(favicon_png) END,
       favicon_updated_at,
       updated_at
FROM viewer_site_settings
WHERE id = 1`).Scan(&settings.SiteName, &faviconBytes, &faviconUpdatedAt, &settings.UpdatedAt)
	if err != nil {
		return SiteSettings{}, err
	}
	settings.HasFavicon = faviconBytes > 0
	if faviconUpdatedAt.Valid {
		settings.FaviconUpdatedAt = ptrInt64(faviconUpdatedAt.Int64)
	}
	return settings, nil
}

func (s *Service) UpdateSiteName(ctx context.Context, siteName string) (SiteSettings, error) {
	siteName = strings.TrimSpace(siteName)
	if err := validateSiteName(siteName); err != nil {
		return SiteSettings{}, err
	}
	now := s.now().UTC().Unix()
	if _, err := s.db.ExecContext(ctx, `
UPDATE viewer_site_settings
SET site_name = ?, updated_at = ?
WHERE id = 1`, siteName, now); err != nil {
		return SiteSettings{}, err
	}
	return s.SiteSettings(ctx)
}

func (s *Service) UpdateFavicon(ctx context.Context, png []byte) (SiteSettings, error) {
	if !validFaviconPNG(png) {
		return SiteSettings{}, ErrInvalidFavicon
	}
	now := s.now().UTC().Unix()
	if _, err := s.db.ExecContext(ctx, `
UPDATE viewer_site_settings
SET favicon_png = ?, favicon_updated_at = ?, updated_at = ?
WHERE id = 1`, png, now, now); err != nil {
		return SiteSettings{}, err
	}
	return s.SiteSettings(ctx)
}

func (s *Service) Favicon(ctx context.Context) ([]byte, int64, bool, error) {
	var data []byte
	var updatedAt sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT favicon_png, favicon_updated_at
FROM viewer_site_settings
WHERE id = 1`).Scan(&data, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	if len(data) == 0 {
		return nil, 0, false, nil
	}
	if updatedAt.Valid {
		return data, updatedAt.Int64, true, nil
	}
	return data, 0, true, nil
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

func validateSiteName(siteName string) error {
	length := utf8.RuneCountInString(siteName)
	if length < 1 || length > 80 {
		return ErrInvalidSiteName
	}
	for _, r := range siteName {
		if unicode.IsControl(r) {
			return ErrInvalidSiteName
		}
	}
	return nil
}

func validFaviconPNG(data []byte) bool {
	return len(data) > 0 && len(data) <= 1<<20 && bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
}

func scanManagedUser(ctx context.Context, tx *sql.Tx, userID int64) (ManagedUser, error) {
	var user ManagedUser
	var disabledAt sql.NullInt64
	err := tx.QueryRowContext(ctx, `
SELECT id, username, disabled_at, created_at, updated_at
FROM viewer_users
WHERE id = ?`, userID).Scan(&user.ID, &user.Username, &disabledAt, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ManagedUser{}, ErrUserNotFound
	}
	if err != nil {
		return ManagedUser{}, err
	}
	if disabledAt.Valid {
		user.Disabled = true
		user.DisabledAt = ptrInt64(disabledAt.Int64)
	}
	return user, nil
}

func ptrInt64(value int64) *int64 {
	return &value
}
