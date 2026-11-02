package service

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/jatinjkagrawal/user-management-poc/internal/auth"
	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
	"github.com/jatinjkagrawal/user-management-poc/internal/platform/mailer"
	"github.com/jatinjkagrawal/user-management-poc/internal/repository"
)

// OnboardingService creates invitation tokens and sends invitation emails.
type OnboardingService struct{ d Deps }

// PendingInvite is produced inside a transaction and delivered after commit,
// so we never email a link for a user that was rolled back.
type PendingInvite struct {
	User       *domain.User
	TenantName string
	Link       string
}

// IssueInvite revokes any outstanding onboarding tokens of the user and
// creates a fresh one. Must be called within a transaction.
func (s *OnboardingService) IssueInvite(ctx context.Context, tx *repository.Store, tenant *domain.Tenant, user *domain.User) (*PendingInvite, error) {
	if err := tx.Tokens.InvalidateActionTokens(ctx, tenant.ID, user.ID, domain.PurposeOnboarding); err != nil {
		return nil, err
	}
	plain, hash, err := auth.NewOpaqueToken()
	if err != nil {
		return nil, err
	}
	exp := time.Now().Add(s.d.Config.Onboard.InviteTTL)
	if err := tx.Tokens.CreateActionToken(ctx, tenant.ID, user.ID, domain.PurposeOnboarding, hash, exp); err != nil {
		return nil, err
	}
	link := fmt.Sprintf("%s/api/v1/auth/onboarding/verify?token=%s", s.d.Config.App.PublicBaseURL, url.QueryEscape(plain))
	return &PendingInvite{User: user, TenantName: tenant.Name, Link: link}, nil
}

// Deliver sends the invitation email. Failures are logged and reported to the
// caller but do not undo the invitation; it can be re-sent.
func (s *OnboardingService) Deliver(ctx context.Context, inv *PendingInvite) bool {
	msg, err := mailer.InvitationEmail(inv.User.Email, mailer.InvitationData{
		Name:       inv.User.FullName(),
		TenantName: inv.TenantName,
		Link:       inv.Link,
		ExpiresIn:  s.d.Config.Onboard.InviteTTL.String(),
	})
	if err == nil {
		sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		err = s.d.Mailer.Send(sendCtx, msg)
	}
	if err != nil {
		s.d.Log.Error("failed to send invitation email",
			"user_id", inv.User.ID, "tenant_id", inv.User.TenantID, "error", err)
		return false
	}
	s.d.Log.Info("invitation email sent", "user_id", inv.User.ID, "tenant_id", inv.User.TenantID)
	return true
}
