package viewer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *Service) UserInvitationCodes(ctx context.Context, userID int64) (UserInvitationOverview, error) {
	createdAt, err := s.userInvitationRegistrationTime(ctx, s.db, userID)
	if err != nil {
		return UserInvitationOverview{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT invites.id, invites.code, invites.used_by_user_id, used_users.username, invites.used_at, invites.created_at,
       invites.created_by_user_id, creator_users.username
FROM viewer_invitation_codes AS invites
LEFT JOIN viewer_users AS used_users ON used_users.id = invites.used_by_user_id
LEFT JOIN viewer_users AS creator_users ON creator_users.id = invites.created_by_user_id
WHERE invites.created_by_user_id = ?
ORDER BY invites.created_at DESC, invites.id DESC`, userID)
	if err != nil {
		return UserInvitationOverview{}, err
	}
	items := make([]InvitationCode, 0, MaxUserInvitationCodes)
	for rows.Next() {
		code, scanErr := scanInvitationCodeRows(rows)
		if scanErr != nil {
			rows.Close()
			return UserInvitationOverview{}, scanErr
		}
		items = append(items, code)
	}
	if err := rows.Close(); err != nil {
		return UserInvitationOverview{}, err
	}
	if err := rows.Err(); err != nil {
		return UserInvitationOverview{}, err
	}
	return UserInvitationOverview{
		Items: items,
		Allowance: calculateUserInvitationAllowance(
			time.Unix(createdAt, 0).UTC(), s.now().UTC(), len(items),
		),
	}, nil
}

func (s *Service) GenerateUserInvitationCode(ctx context.Context, userID int64) (InvitationCode, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return InvitationCode{}, err
	}
	defer tx.Rollback()

	createdAt, err := s.userInvitationRegistrationTime(ctx, tx, userID)
	if err != nil {
		return InvitationCode{}, err
	}
	var createdCount int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM viewer_invitation_codes WHERE created_by_user_id = ?",
		userID,
	).Scan(&createdCount); err != nil {
		return InvitationCode{}, err
	}
	now := s.now().UTC()
	allowance := calculateUserInvitationAllowance(time.Unix(createdAt, 0).UTC(), now, createdCount)
	if !allowance.CanCreate {
		return InvitationCode{}, ErrInvitationQuotaReached
	}

	var creatorUsername string
	if err := tx.QueryRowContext(ctx, "SELECT username FROM viewer_users WHERE id = ?", userID).Scan(&creatorUsername); err != nil {
		return InvitationCode{}, err
	}
	for attempt := 0; attempt < 12; attempt++ {
		code, err := randomInviteCode()
		if err != nil {
			return InvitationCode{}, err
		}
		result, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO viewer_invitation_codes(code, created_by_user_id, created_at)
VALUES (?, ?, ?)`, code, userID, now.Unix())
		if err != nil {
			return InvitationCode{}, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return InvitationCode{}, err
		}
		if affected == 0 {
			continue
		}
		id, err := result.LastInsertId()
		if err != nil {
			return InvitationCode{}, err
		}
		if err := tx.Commit(); err != nil {
			return InvitationCode{}, err
		}
		creatorID := userID
		return InvitationCode{
			ID: id, Code: code, CreatedAt: now.Unix(),
			CreatedByUserID: &creatorID, CreatedByUsername: creatorUsername,
		}, nil
	}
	return InvitationCode{}, fmt.Errorf("generate unique invite code: too many collisions")
}

type invitationUserQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Service) userInvitationRegistrationTime(ctx context.Context, query invitationUserQuerier, userID int64) (int64, error) {
	var createdAt int64
	var disabledAt sql.NullInt64
	err := query.QueryRowContext(ctx,
		"SELECT created_at, disabled_at FROM viewer_users WHERE id = ?",
		userID,
	).Scan(&createdAt, &disabledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrUnauthorized
	}
	if err != nil {
		return 0, err
	}
	if disabledAt.Valid {
		return 0, ErrUserDisabled
	}
	return createdAt, nil
}

func calculateUserInvitationAllowance(registeredAt, now time.Time, createdCount int) UserInvitationAllowance {
	registeredAt = registeredAt.UTC()
	now = now.UTC()
	eligibleTotal := 0
	var nextEligibleAt *int64
	firstEligibleAt := registeredAt.Add(7 * 24 * time.Hour)
	if now.Before(firstEligibleAt) {
		next := firstEligibleAt.Unix()
		nextEligibleAt = &next
	} else {
		eligibleTotal = 1
		for month := 1; month < MaxUserInvitationCodes; month++ {
			eligibleAt := addCalendarMonths(registeredAt, month)
			if now.Before(eligibleAt) {
				next := eligibleAt.Unix()
				nextEligibleAt = &next
				break
			}
			eligibleTotal++
		}
	}
	if eligibleTotal > MaxUserInvitationCodes {
		eligibleTotal = MaxUserInvitationCodes
	}
	if eligibleTotal == MaxUserInvitationCodes {
		nextEligibleAt = nil
	}
	remaining := eligibleTotal - createdCount
	if remaining < 0 {
		remaining = 0
	}
	return UserInvitationAllowance{
		EligibleTotal: eligibleTotal, CreatedCount: createdCount, RemainingCount: remaining,
		MaximumTotal: MaxUserInvitationCodes, CanCreate: remaining > 0, NextEligibleAt: nextEligibleAt,
	}
}

func addCalendarMonths(value time.Time, months int) time.Time {
	year, month, day := value.Date()
	targetMonth := time.Date(year, month+time.Month(months), 1, value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), value.Location())
	lastDay := time.Date(targetMonth.Year(), targetMonth.Month()+1, 0, 0, 0, 0, 0, value.Location()).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(targetMonth.Year(), targetMonth.Month(), day, value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), value.Location())
}
