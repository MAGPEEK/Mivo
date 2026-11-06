package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// Resolve turns a session token into the session, account, profile and viewer. Unknown,
// expired or revoked tokens and disabled accounts return an error wrapping
// domain.ErrUnauthenticated. Results are cached for CacheTTL; sign-out, revoke, password
// change and EvictAccount drop them earlier.
func (service *Service) Resolve(ctx context.Context, token string, peer Peer) (Resolution, error) {
	if token == "" {
		return Resolution{}, unauthenticated("no token")
	}

	tokenHash := service.deps.Tokens.HashToken(token)
	now := service.deps.Clock()

	if cached, ok := service.cache.get(tokenHash, now); ok && !expired(cached.Session, now) {
		return cached, nil
	}

	generation := service.cache.currentGeneration()

	resolution, err := service.load(ctx, tokenHash, now)
	if err != nil {
		return Resolution{}, err
	}

	service.touch(ctx, &resolution.Session, peer, now)
	service.cache.put(tokenHash, resolution, now, generation)

	return resolution, nil
}

func (service *Service) load(ctx context.Context, tokenHash string, now time.Time) (Resolution, error) {
	session, err := service.deps.Sessions.SessionByTokenHash(ctx, tokenHash)
	if errors.Is(err, domain.ErrNotFound) {
		return Resolution{}, unauthenticated("unknown session")
	}

	if err != nil {
		return Resolution{}, fmt.Errorf("resolve session: %w", err)
	}

	if expired(session, now) {
		return Resolution{}, unauthenticated("session expired")
	}

	if selectionLapsed(session, now) {
		service.clearLapsedSelection(ctx, &session)
	}

	inputs, err := service.deps.Profiles.ViewerInputs(ctx, session.AccountID, session.ProfileID)
	if errors.Is(err, domain.ErrNotFound) {
		return Resolution{}, unauthenticated("account or profile gone")
	}

	if err != nil {
		return Resolution{}, fmt.Errorf("resolve session: load viewer: %w", err)
	}

	if err := checkInputs(session, inputs); err != nil {
		return Resolution{}, err
	}

	selected := session.ProfileID != ""

	viewer, err := buildViewer(session.ID, inputs, selected)
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve session: %w", err)
	}

	return Resolution{
		Session:         session,
		Account:         inputs.Account,
		Profile:         inputs.Profile,
		ProfileSelected: selected,
		Viewer:          viewer,
	}, nil
}

// checkInputs refuses a disabled account and inputs that do not belong to the session, so a
// faulty store cannot hand out another account's access.
func checkInputs(session Session, inputs ViewerInputs) error {
	if inputs.Account.ID != session.AccountID {
		return unauthenticated("account mismatch")
	}

	if inputs.Account.Disabled {
		return unauthenticated("account disabled")
	}

	if session.ProfileID != "" &&
		(inputs.Profile.ID != session.ProfileID || inputs.Profile.AccountID != session.AccountID) {
		return unauthenticated("profile mismatch")
	}

	return nil
}

// touch writes last_seen at most every TouchInterval, or when the address class changed. A
// failed write only delays the idle timeout's slide, so it is logged and the request goes on.
func (service *Service) touch(ctx context.Context, session *Session, peer Peer, now time.Time) {
	classChanged := peer.Class != "" && peer.Class != session.AddressClass
	if !classChanged && now.Sub(session.LastSeenAt) < TouchInterval {
		return
	}

	seen := Seen{At: now, AddressClass: session.AddressClass}
	if peer.Class != "" {
		seen.AddressClass = peer.Class
	}

	if err := service.deps.Sessions.TouchSession(ctx, session.ID, seen); err != nil {
		service.deps.Logger.WarnContext(ctx, "touch session", "session", session.ID, "error", err)

		return
	}

	session.LastSeenAt, session.AddressClass = seen.At, seen.AddressClass
}

// expired applies the idle timeout and the absolute lifetime of ADR 0032 §3.
func expired(session Session, now time.Time) bool {
	return now.Sub(session.LastSeenAt) >= IdleTimeout || now.Sub(session.CreatedAt) >= AbsoluteLifetime
}

// selectionLapsed reports a selection without remember that was not used for SelectionLapse.
// LastSeenAt lags by up to TouchInterval, far below the lapse.
func selectionLapsed(session Session, now time.Time) bool {
	return session.ProfileID != "" && !session.Remember && now.Sub(session.LastSeenAt) >= SelectionLapse
}

// clearLapsedSelection resolves the session without its profile, so the client shows the
// picker again. A failed store write is logged only: this request still fails closed, and
// the next one finds the lapse again.
func (service *Service) clearLapsedSelection(ctx context.Context, session *Session) {
	session.ProfileID = ""

	if err := service.deps.Selector.SetSessionProfile(ctx, session.ID, "", false); err != nil {
		service.deps.Logger.WarnContext(ctx, "clear lapsed profile selection", "session", session.ID, "error", err)
	}
}

func unauthenticated(reason string) error {
	return fmt.Errorf("resolve session: %s: %w", reason, domain.ErrUnauthenticated)
}

// EvictSession drops the cached resolution of one session, for example after a device was
// revoked (M7-16).
func (service *Service) EvictSession(sessionID string) {
	service.cache.evict(func(resolution Resolution) bool { return resolution.Session.ID == sessionID })
}

// EvictAccount drops every cached resolution of an account, for example after it was
// disabled or its role changed (M7-17).
func (service *Service) EvictAccount(accountID string) {
	service.cache.evict(func(resolution Resolution) bool { return resolution.Session.AccountID == accountID })
}

// PurgeExpired deletes expired sessions and forgets stale cache and throttle entries. It is
// meant for a periodic call from cmd/mivo and returns the number of deleted sessions.
func (service *Service) PurgeExpired(ctx context.Context) (int64, error) {
	now := service.deps.Clock()

	deleted, err := service.deps.Revoker.PurgeSessions(ctx, now.Add(-IdleTimeout), now.Add(-AbsoluteLifetime))
	if err != nil {
		return 0, fmt.Errorf("purge expired sessions: %w", err)
	}

	service.cache.prune(now)
	service.throttle.prune(now)
	service.pinLock.prune(now)

	return deleted, nil
}

// RevokeAccountSessions deletes every session of an account and drops them from the cache,
// for example after an admin password reset or a disable (M7-17).
func (service *Service) RevokeAccountSessions(ctx context.Context, accountID string) error {
	// Evicted before and after the delete for the same reason as in replacePassword.
	service.EvictAccount(accountID)

	if err := service.deps.Revoker.DeleteOtherSessions(ctx, accountID, ""); err != nil {
		return fmt.Errorf("revoke account sessions: %w", err)
	}

	service.EvictAccount(accountID)

	return nil
}
