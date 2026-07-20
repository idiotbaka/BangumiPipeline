package viewer

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"bangumipipeline.local/server/internal/database"

	"github.com/SherClockHolmes/webpush-go"
)

const PushDeliveryTaskKey = "deliver-viewer-push-notifications"

const (
	pushDeliveryPending   = "pending"
	pushDeliveryDelivered = "delivered"
	pushDeliveryFailed    = "failed"
	pushDeliveryLimit     = 100
	pushDeliveryMaxTries  = 3
)

var ErrInvalidPushSubscription = errors.New("invalid web push subscription")

type PushSubscriptionInput struct {
	Endpoint       string `json:"endpoint"`
	ExpirationTime *int64 `json:"expirationTime"`
	Keys           struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type PushConfig struct {
	Supported bool   `json:"supported"`
	PublicKey string `json:"publicKey"`
}

type PushService struct {
	db      database.Executor
	logger  *slog.Logger
	contact string
	now     func() time.Time
	sender  pushSender

	keyMu      sync.Mutex
	deliveryMu sync.Mutex
}

type pushSender interface {
	Send(context.Context, pushSubscription, []byte, vapidKeys) (int, error)
}

type pushSubscription struct {
	ID       int64
	Endpoint string
	P256dh   string
	Auth     string
}

type vapidKeys struct {
	Public  string
	Private string
}

type pendingPushDelivery struct {
	ID       int64
	Attempts int
	pushSubscription
	MediaID       int64
	BangumiID     int64
	AnimeTitle    string
	SeasonNumber  int
	EpisodeType   string
	EpisodeNumber string
}

type browserPushSender struct {
	client  *http.Client
	contact string
}

func NewPushService(db database.Executor, logger *slog.Logger, contactEmail string) *PushService {
	contactEmail = strings.TrimSpace(contactEmail)
	if contactEmail == "" {
		contactEmail = "noreply@localhost"
	}
	return &PushService{
		db: db, logger: logger, contact: contactEmail, now: time.Now,
		sender: browserPushSender{
			client:  &http.Client{Timeout: 10 * time.Second},
			contact: contactEmail,
		},
	}
}

func (s *PushService) Config(ctx context.Context) (PushConfig, error) {
	keys, err := s.vapidKeys(ctx)
	if err != nil {
		return PushConfig{}, err
	}
	return PushConfig{Supported: true, PublicKey: keys.Public}, nil
}

func (s *PushService) UpsertSubscription(ctx context.Context, userID int64, input PushSubscriptionInput) error {
	if userID < 1 || !validPushSubscription(input) {
		return ErrInvalidPushSubscription
	}
	if _, err := s.vapidKeys(ctx); err != nil {
		return err
	}
	now := s.now().UTC().Unix()
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO viewer_web_push_subscriptions(
    user_id, endpoint, p256dh, auth, expires_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(endpoint) DO UPDATE SET
    user_id = excluded.user_id,
    p256dh = excluded.p256dh,
    auth = excluded.auth,
    expires_at = excluded.expires_at,
    updated_at = excluded.updated_at`,
		userID, strings.TrimSpace(input.Endpoint), strings.TrimSpace(input.Keys.P256dh),
		strings.TrimSpace(input.Keys.Auth), input.ExpirationTime, now, now,
	); err != nil {
		return err
	}
	var subscriptionID int64
	if err := s.db.QueryRowContext(ctx, `
SELECT id FROM viewer_web_push_subscriptions WHERE endpoint = ?`, strings.TrimSpace(input.Endpoint),
	).Scan(&subscriptionID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
DELETE FROM viewer_web_push_deliveries
WHERE subscription_id = ? AND status != ?`, subscriptionID, pushDeliveryDelivered)
	return err
}

func (s *PushService) RemoveSubscription(ctx context.Context, userID int64, endpoint string) error {
	if userID < 1 {
		return ErrInvalidPushSubscription
	}
	_, err := s.db.ExecContext(ctx, `
DELETE FROM viewer_web_push_subscriptions
WHERE user_id = ? AND endpoint = ?`, userID, strings.TrimSpace(endpoint))
	return err
}

func (s *PushService) NotifyMediaCompleted(ctx context.Context, mediaID, bangumiID int64) error {
	if mediaID < 1 || bangumiID < 1 {
		return nil
	}
	now := s.now().UTC().Unix()
	if _, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO viewer_web_push_deliveries(
    subscription_id, media_job_id, status, attempts, next_attempt_at, created_at, updated_at
)
SELECT subscription.id, ?, ?, 0, ?, ?, ?
FROM viewer_anime_follows follow
JOIN viewer_web_push_subscriptions subscription ON subscription.user_id = follow.user_id
WHERE follow.bangumi_id = ?`,
		mediaID, pushDeliveryPending, now, now, now, bangumiID,
	); err != nil {
		return err
	}
	return s.deliverPending(ctx, pushDeliveryLimit)
}

func (s *PushService) Execute(ctx context.Context) error {
	return s.deliverPending(ctx, pushDeliveryLimit)
}

func (s *PushService) deliverPending(ctx context.Context, limit int) error {
	if limit < 1 {
		return nil
	}
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()

	deliveries, err := s.pendingDeliveries(ctx, limit)
	if err != nil || len(deliveries) == 0 {
		return err
	}
	keys, err := s.vapidKeys(ctx)
	if err != nil {
		return err
	}
	for _, delivery := range deliveries {
		payload, err := json.Marshal(pushPayloadFor(delivery))
		if err != nil {
			return err
		}
		statusCode, sendErr := s.sender.Send(ctx, delivery.pushSubscription, payload, keys)
		if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices && sendErr == nil {
			if err := s.markDeliveryDelivered(ctx, delivery.ID); err != nil {
				return err
			}
			s.logger.Info("观看端新集通知已发送", "source", "viewer", "media_job_id", delivery.MediaID, "subscription_id", delivery.pushSubscription.ID)
			continue
		}
		if statusCode == http.StatusNotFound || statusCode == http.StatusGone {
			if err := s.removeInvalidSubscription(ctx, delivery.pushSubscription.ID); err != nil {
				return err
			}
			s.logger.Info("移除失效的观看端推送订阅", "source", "viewer", "subscription_id", delivery.pushSubscription.ID, "status", statusCode)
			continue
		}
		if err := s.markDeliveryRetry(ctx, delivery.ID, delivery.Attempts, pushSendFailureMessage(statusCode, sendErr)); err != nil {
			return err
		}
		s.logger.Warn("观看端新集通知发送失败", "source", "viewer", "media_job_id", delivery.MediaID, "subscription_id", delivery.pushSubscription.ID, "status", statusCode, "error", sendErr)
	}
	return nil
}

func (s *PushService) pendingDeliveries(ctx context.Context, limit int) ([]pendingPushDelivery, error) {
	now := s.now().UTC().Unix()
	rows, err := s.db.QueryContext(ctx, `
SELECT delivery.id, delivery.attempts,
       subscription.id, subscription.endpoint, subscription.p256dh, subscription.auth,
       media.id, media.bangumi_id,
       COALESCE(NULLIF(anime.name_cn, ''), NULLIF(anime.name, ''), NULLIF(media.anime_name, ''), '番剧'),
       media.season_number, COALESCE(NULLIF(media.episode_type, ''), 'episode'), media.episode_number
FROM viewer_web_push_deliveries delivery
JOIN viewer_web_push_subscriptions subscription ON subscription.id = delivery.subscription_id
JOIN media_jobs media ON media.id = delivery.media_job_id
LEFT JOIN anime_metadata anime ON anime.bangumi_id = media.bangumi_id
WHERE delivery.status = ? AND delivery.next_attempt_at <= ?
  AND media.status = 'completed' AND media.output_path != ''
ORDER BY delivery.id
LIMIT ?`, pushDeliveryPending, now, limit)
	if err != nil {
		return nil, err
	}
	items := make([]pendingPushDelivery, 0)
	for rows.Next() {
		var item pendingPushDelivery
		if err := rows.Scan(
			&item.ID, &item.Attempts, &item.pushSubscription.ID, &item.Endpoint,
			&item.P256dh, &item.Auth, &item.MediaID, &item.BangumiID, &item.AnimeTitle,
			&item.SeasonNumber, &item.EpisodeType, &item.EpisodeNumber,
		); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return items, rows.Err()
}

func (s *PushService) markDeliveryDelivered(ctx context.Context, deliveryID int64) error {
	now := s.now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, `
UPDATE viewer_web_push_deliveries
SET status = ?, delivered_at = ?, error_message = '', updated_at = ?
WHERE id = ?`, pushDeliveryDelivered, now, now, deliveryID)
	return err
}

func (s *PushService) markDeliveryRetry(ctx context.Context, deliveryID int64, previousAttempts int, message string) error {
	attempts := previousAttempts + 1
	now := s.now().UTC()
	status := pushDeliveryPending
	nextAttemptAt := now.Add(time.Duration(attempts*5) * time.Minute).Unix()
	if attempts >= pushDeliveryMaxTries {
		status = pushDeliveryFailed
		nextAttemptAt = now.Unix()
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE viewer_web_push_deliveries
SET status = ?, attempts = ?, next_attempt_at = ?, error_message = ?, updated_at = ?
WHERE id = ?`, status, attempts, nextAttemptAt, limitPushError(message), now.Unix(), deliveryID)
	return err
}

func (s *PushService) removeInvalidSubscription(ctx context.Context, subscriptionID int64) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM viewer_web_push_subscriptions WHERE id = ?`, subscriptionID)
	return err
}

func (s *PushService) vapidKeys(ctx context.Context) (vapidKeys, error) {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()

	var keys vapidKeys
	err := s.db.QueryRowContext(ctx, `
SELECT vapid_public_key, vapid_private_key
FROM viewer_web_push_settings
WHERE id = 1`).Scan(&keys.Public, &keys.Private)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return vapidKeys{}, err
	}
	if err == nil && strings.TrimSpace(keys.Public) != "" && strings.TrimSpace(keys.Private) != "" {
		return keys, nil
	}
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return vapidKeys{}, fmt.Errorf("generate VAPID keys: %w", err)
	}
	now := s.now().UTC().Unix()
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO viewer_web_push_settings(id, vapid_public_key, vapid_private_key, created_at, updated_at)
VALUES (1, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    vapid_public_key = excluded.vapid_public_key,
    vapid_private_key = excluded.vapid_private_key,
    updated_at = excluded.updated_at`, publicKey, privateKey, now, now); err != nil {
		return vapidKeys{}, err
	}
	return vapidKeys{Public: publicKey, Private: privateKey}, nil
}

func (s browserPushSender) Send(ctx context.Context, subscription pushSubscription, payload []byte, keys vapidKeys) (int, error) {
	response, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: subscription.Endpoint,
		Keys: webpush.Keys{
			P256dh: subscription.P256dh,
			Auth:   subscription.Auth,
		},
	}, &webpush.Options{
		HTTPClient:      s.client,
		Subscriber:      s.contact,
		TTL:             3600,
		Urgency:         webpush.UrgencyHigh,
		VAPIDPublicKey:  keys.Public,
		VAPIDPrivateKey: keys.Private,
	})
	if response == nil {
		return 0, err
	}
	_ = response.Body.Close()
	return response.StatusCode, err
}

func validPushSubscription(input PushSubscriptionInput) bool {
	endpoint, err := url.ParseRequestURI(strings.TrimSpace(input.Endpoint))
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil {
		return false
	}
	host := endpoint.Hostname()
	if host == "" || net.ParseIP(host) != nil || strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".local") {
		return false
	}
	p256dh, err := decodePushKey(input.Keys.P256dh)
	if err != nil || len(p256dh) != 65 {
		return false
	}
	auth, err := decodePushKey(input.Keys.Auth)
	return err == nil && len(auth) >= 16 && len(auth) <= 64
}

func decodePushKey(value string) ([]byte, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "=")
	if value == "" {
		return nil, ErrInvalidPushSubscription
	}
	return base64.RawURLEncoding.DecodeString(value)
}

func pushPayloadFor(delivery pendingPushDelivery) map[string]any {
	episodeLabel := watchEpisodeLabel(delivery.SeasonNumber, delivery.EpisodeType, delivery.EpisodeNumber)
	return map[string]any{
		"title": fmt.Sprintf("《%s》更新了", delivery.AnimeTitle),
		"body":  fmt.Sprintf("%s 现已可播放", episodeLabel),
		"tag":   fmt.Sprintf("bangumi-%d", delivery.BangumiID),
		"url":   fmt.Sprintf("/anime/%d?media=%d", delivery.BangumiID, delivery.MediaID),
	}
}

func pushSendFailureMessage(statusCode int, err error) string {
	if err != nil {
		return err.Error()
	}
	if statusCode > 0 {
		return fmt.Sprintf("push service returned HTTP %d", statusCode)
	}
	return "push service did not return a response"
}

func limitPushError(message string) string {
	if len(message) > 1000 {
		return message[:1000]
	}
	return message
}
