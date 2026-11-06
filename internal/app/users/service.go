// Package users holds the user and profile administration use cases (ADR 0032 §1, §7, §9;
// US-10, US-11, US-20, US-22): admins manage accounts, their library access and limits;
// account owners manage their managed profiles. The domain rules come from domain/account
// (who may manage what, the last-admin rule, the managed profile cap) and domain/access
// (library sets and content limits); every rule that depends on other rows runs inside the
// store transaction through the change and check callbacks in the ports.
package users

import (
	"context"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// Options are the collaborators of the Service; all are required.
type Options struct {
	Accounts AccountReader
	Writer   AccountWriter
	Password PasswordWriter
	Profiles ProfileReader
	Profile  ProfileWriter
	Hasher   Hasher
	IDs      IDSource
	Sessions SessionRevoker
	Cache    ResolveCache
	Viewers  ViewerSource
	// Compose turns names into Unicode NFC; cmd/mivo passes norm.NFC.String, because the
	// inner layers may import only the standard library (BACKLOG "m7-04 notes").
	Compose account.Composer
}

// Service runs the administration use cases for the viewer in the context.
type Service struct {
	options Options
}

// errMissingDependency reports an incomplete Options value.
var errMissingDependency = errors.New("users: missing dependency")

// New returns the service or an error naming the first missing collaborator.
func New(options Options) (*Service, error) {
	required := []struct {
		name    string
		present bool
	}{
		{"Accounts", options.Accounts != nil},
		{"Writer", options.Writer != nil},
		{"Password", options.Password != nil},
		{"Profiles", options.Profiles != nil},
		{"Profile", options.Profile != nil},
		{"Hasher", options.Hasher != nil},
		{"IDs", options.IDs != nil},
		{"Sessions", options.Sessions != nil},
		{"Cache", options.Cache != nil},
		{"Viewers", options.Viewers != nil},
		{"Compose", options.Compose != nil},
	}

	for _, dependency := range required {
		if !dependency.present {
			return nil, fmt.Errorf("%w: %s", errMissingDependency, dependency.name)
		}
	}

	return &Service{options: options}, nil
}

// viewer returns the request's viewer; without one the use case fails closed.
func (s *Service) viewer(ctx context.Context) (access.Viewer, error) {
	viewer, ok := s.options.Viewers(ctx)
	if !ok {
		return access.Viewer{}, fmt.Errorf("users: no viewer: %w", domain.ErrUnauthenticated)
	}

	return viewer, nil
}

// requireAdmin refuses every viewer without admin powers (ADR 0032 §9).
func (s *Service) requireAdmin(ctx context.Context) error {
	viewer, err := s.viewer(ctx)
	if err != nil {
		return err
	}

	if !viewer.Admin {
		return fmt.Errorf("users: viewer %s is not an admin: %w", viewer.AccountID, domain.ErrForbidden)
	}

	return nil
}

// actor loads the viewer's account and selected profile fresh from the store, so the
// ownership rule sees the current role and disabled state.
func (s *Service) actor(ctx context.Context) (account.Actor, error) {
	viewer, err := s.viewer(ctx)
	if err != nil {
		return account.Actor{}, err
	}

	current, err := s.options.Accounts.GetAccount(ctx, viewer.AccountID)
	if err != nil {
		return account.Actor{}, fmt.Errorf("load viewer account %s: %w", viewer.AccountID, err)
	}

	selected, err := s.options.Profiles.GetProfile(ctx, viewer.ProfileID)
	if err != nil {
		return account.Actor{}, fmt.Errorf("load viewer profile %s: %w", viewer.ProfileID, err)
	}

	return account.Actor{Account: current.Account, Profile: selected.Profile}, nil
}

// normalizeName composes and validates a display name.
func (s *Service) normalizeName(raw string) (string, error) {
	name, err := account.NormalizeName(raw, s.options.Compose)
	if err != nil {
		return "", fmt.Errorf("normalize name: %w", err)
	}

	return name, nil
}

// newID returns a new id for an account or profile.
func (s *Service) newID() (string, error) {
	id, err := s.options.IDs.NewID()
	if err != nil {
		return "", fmt.Errorf("new id: %w", err)
	}

	return id, nil
}

// hash returns the encoded hash of a secret the caller has already validated.
func (s *Service) hash(ctx context.Context, secret string) (string, error) {
	encoded, err := s.options.Hasher.Hash(ctx, secret)
	if err != nil {
		return "", fmt.Errorf("hash secret: %w", err)
	}

	return encoded, nil
}
