package viewer

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"bangumipipeline.local/server/internal/database"
)

func TestCalculateUserInvitationAllowance(t *testing.T) {
	registeredAt := time.Date(2026, time.January, 31, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		now           time.Time
		createdCount  int
		eligibleTotal int
		remaining     int
		canCreate     bool
		nextEligible  time.Time
	}{
		{
			name: "before first week", now: registeredAt.Add(7*24*time.Hour - time.Second),
			eligibleTotal: 0, remaining: 0, nextEligible: registeredAt.Add(7 * 24 * time.Hour),
		},
		{
			name: "first week", now: registeredAt.Add(7 * 24 * time.Hour),
			eligibleTotal: 1, remaining: 1, canCreate: true,
			nextEligible: time.Date(2026, time.February, 28, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "one calendar month with first code used", now: time.Date(2026, time.February, 28, 12, 0, 0, 0, time.UTC), createdCount: 1,
			eligibleTotal: 2, remaining: 1, canCreate: true,
			nextEligible: time.Date(2026, time.March, 31, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "maximum at nine months", now: time.Date(2026, time.October, 31, 12, 0, 0, 0, time.UTC), createdCount: 9,
			eligibleTotal: 10, remaining: 1, canCreate: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allowance := calculateUserInvitationAllowance(registeredAt, test.now, test.createdCount)
			if allowance.EligibleTotal != test.eligibleTotal || allowance.RemainingCount != test.remaining || allowance.CanCreate != test.canCreate {
				t.Fatalf("unexpected allowance: %+v", allowance)
			}
			if allowance.MaximumTotal != MaxUserInvitationCodes || allowance.CreatedCount != test.createdCount {
				t.Fatalf("unexpected allowance totals: %+v", allowance)
			}
			if test.nextEligible.IsZero() {
				if allowance.NextEligibleAt != nil {
					t.Fatalf("expected no next eligibility, got %d", *allowance.NextEligibleAt)
				}
			} else if allowance.NextEligibleAt == nil || *allowance.NextEligibleAt != test.nextEligible.Unix() {
				t.Fatalf("unexpected next eligibility: %+v", allowance.NextEligibleAt)
			}
		})
	}
}

func TestUserInvitationGenerationAndRegistrationOrigins(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "viewer-invitations.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewService(db, time.Hour)
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	if _, err := service.UpdateSiteSettings(ctx, SiteSettingsUpdate{
		SiteName: "Test Viewer", RegistrationEnabled: true, InviteRequired: false,
	}); err != nil {
		t.Fatal(err)
	}
	alice, _, err := service.Register(ctx, "alice-user", "alice-password", "")
	if err != nil {
		t.Fatal(err)
	}
	if alice.RegistrationSource != RegistrationSourceOpen {
		t.Fatalf("unexpected open registration source: %+v", alice)
	}

	now = now.Add(6 * 24 * time.Hour)
	overview, err := service.UserInvitationCodes(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if overview.Allowance.EligibleTotal != 0 || overview.Allowance.CanCreate {
		t.Fatalf("unexpected pre-week allowance: %+v", overview.Allowance)
	}
	if _, err := service.GenerateUserInvitationCode(ctx, alice.ID); !errors.Is(err, ErrInvitationQuotaReached) {
		t.Fatalf("expected pre-week quota rejection, got %v", err)
	}

	now = now.Add(24 * time.Hour)
	userInvite, err := service.GenerateUserInvitationCode(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if userInvite.CreatedByUserID == nil || *userInvite.CreatedByUserID != alice.ID || userInvite.CreatedByUsername != alice.Username {
		t.Fatalf("unexpected user-generated invitation: %+v", userInvite)
	}
	if _, err := service.GenerateUserInvitationCode(ctx, alice.ID); !errors.Is(err, ErrInvitationQuotaReached) {
		t.Fatalf("expected exhausted quota rejection, got %v", err)
	}
	adminInvite, err := service.GenerateInvitationCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if adminInvite.CreatedByUserID != nil || adminInvite.CreatedByUsername != "" {
		t.Fatalf("expected system administrator invitation, got %+v", adminInvite)
	}

	if _, err := service.UpdateSiteSettings(ctx, SiteSettingsUpdate{
		SiteName: "Test Viewer", RegistrationEnabled: true, InviteRequired: true,
	}); err != nil {
		t.Fatal(err)
	}
	bob, bobSession, err := service.Register(ctx, "bob-user", "bob-password", userInvite.Code)
	if err != nil {
		t.Fatal(err)
	}
	if bob.RegistrationSource != RegistrationSourceUserInvite || bob.InvitedByUsername != alice.Username {
		t.Fatalf("unexpected user invitation registration: %+v", bob)
	}
	carol, _, err := service.Register(ctx, "carol-user", "carol-password", adminInvite.Code)
	if err != nil {
		t.Fatal(err)
	}
	if carol.RegistrationSource != RegistrationSourceSystemInvite || carol.InvitedByUsername != "" {
		t.Fatalf("unexpected system invitation registration: %+v", carol)
	}

	authenticatedBob, err := service.Authenticate(ctx, bobSession.Token)
	if err != nil {
		t.Fatal(err)
	}
	if authenticatedBob.RegistrationSource != RegistrationSourceUserInvite || authenticatedBob.InvitedByUsername != alice.Username {
		t.Fatalf("authentication lost invitation origin: %+v", authenticatedBob)
	}
	overview, err = service.UserInvitationCodes(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Items) != 1 || overview.Items[0].UsedByUsername != bob.Username || !overview.Items[0].Used {
		t.Fatalf("unexpected invitation usage list: %+v", overview.Items)
	}
	page, err := service.ListInvitationCodes(ctx, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected two administrator-visible invitations, got %d", len(page.Items))
	}
	var listedUserInvite, listedAdminInvite *InvitationCode
	for index := range page.Items {
		switch page.Items[index].ID {
		case userInvite.ID:
			listedUserInvite = &page.Items[index]
		case adminInvite.ID:
			listedAdminInvite = &page.Items[index]
		}
	}
	if listedUserInvite == nil || listedUserInvite.CreatedByUsername != alice.Username {
		t.Fatalf("administrator list lost user creator: %+v", listedUserInvite)
	}
	if listedAdminInvite == nil || listedAdminInvite.CreatedByUserID != nil || listedAdminInvite.CreatedByUsername != "" {
		t.Fatalf("administrator list did not preserve system creator: %+v", listedAdminInvite)
	}
}
