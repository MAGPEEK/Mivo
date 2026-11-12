package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/magpeek/mivo/internal/adapters/credentials"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/server"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/adapters/uuidgen"
	appaccess "github.com/magpeek/mivo/internal/app/access"
	appauth "github.com/magpeek/mivo/internal/app/auth"
	appusers "github.com/magpeek/mivo/internal/app/users"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// sessionPurgeInterval is how often expired sessions are deleted (ADR 0032 §3). Expired
// sessions are refused on use anyway, so the purge only keeps the table small.
const sessionPurgeInterval = time.Hour

// accountServices are the sign-in and user administration use cases with the account store
// they share.
type accountServices struct {
	auth  *appauth.Service
	users *appusers.Service
	store *sqlite.AccountStore
}

// authentication is what the HTTP server needs to answer for sessions and the setup state.
func (services accountServices) authentication() server.Authentication {
	return server.Authentication{Sessions: services.auth, Setup: services.auth}
}

// userServices are the services of the user administration endpoints.
func (services accountServices) userServices() restapi.UserServices {
	return restapi.UserServices{Accounts: services.users, Profiles: services.users, Devices: services.auth}
}

// newAccountServices builds app/auth and app/users on the sqlite stores. The users service
// reaches the auth service for the two things only it can do: forgetting cached sessions of
// an account and signing the account out of every device.
func newAccountServices(db *sqlite.DB, logger *slog.Logger) (accountServices, error) {
	accounts := sqlite.NewAccountStore(db, time.Now)
	sessions := sqlite.NewSessionStore(db)
	hasher := credentials.NewHasher()
	ids := uuidgen.New()

	authService, err := appauth.NewService(appauth.Dependencies{
		Setup: accountPorts{accounts}, Credentials: accountPorts{accounts}, Profiles: accountPorts{accounts},
		Sessions: sessions, Revoker: sessions, PINs: accounts, Selector: sessions, Devices: sessions,
		Hasher: hasher, Tokens: credentials.NewTokens(), IDs: ids, Clock: time.Now,
		Compose: norm.NFC.String, Logger: logger,
	})
	if err != nil {
		return accountServices{}, fmt.Errorf("create auth service: %w", err)
	}

	usersService, err := appusers.New(appusers.Options{
		Accounts: accounts, Writer: accounts, Password: accounts, Profiles: accounts, Profile: accounts,
		Hasher: hasher, IDs: ids, Sessions: authService, Cache: authService,
		Viewers: appaccess.ViewerFrom, Compose: norm.NFC.String,
	})
	if err != nil {
		return accountServices{}, fmt.Errorf("create users service: %w", err)
	}

	return accountServices{auth: authService, users: usersService, store: accounts}, nil
}

// accountPorts adapts the account store to the setup, credential and profile ports of
// app/auth, whose records differ from the store's. The other store methods those ports and
// app/users need are promoted through the pointer.
type accountPorts struct {
	*sqlite.AccountStore
}

// CreateFirstAdmin stores the first admin, who sees every library and has no content limit.
func (ports accountPorts) CreateFirstAdmin(ctx context.Context, admin appauth.FirstAdmin) error {
	err := ports.AccountStore.CreateFirstAdmin(ctx, sqlite.NewAccountRecord{
		Account: sqlite.AccountRecord{
			Account: admin.Account, Libraries: access.AllLibraries(), PrimaryLimit: access.NoLimit(),
		},
		Primary:      admin.Profile,
		PasswordHash: admin.PasswordHash,
	})
	if err != nil {
		return fmt.Errorf("store first admin: %w", err)
	}

	return nil
}

// CredentialsByName finds the account whose name key is nameKey.
func (ports accountPorts) CredentialsByName(ctx context.Context, nameKey string) (appauth.Credentials, error) {
	found, err := ports.CredentialsByNameKey(ctx, nameKey)
	if err != nil {
		return appauth.Credentials{}, fmt.Errorf("credentials by name: %w", err)
	}

	return appauth.Credentials{Account: found.Record.Account, PasswordHash: found.PasswordHash}, nil
}

// CredentialsByID finds the account with accountID.
func (ports accountPorts) CredentialsByID(ctx context.Context, accountID string) (appauth.Credentials, error) {
	found, err := ports.AccountStore.CredentialsByID(ctx, accountID)
	if err != nil {
		return appauth.Credentials{}, fmt.Errorf("credentials by id: %w", err)
	}

	return appauth.Credentials{Account: found.Record.Account, PasswordHash: found.PasswordHash}, nil
}

// Profiles lists the account's profiles, the primary first.
func (ports accountPorts) Profiles(ctx context.Context, accountID string) ([]account.Profile, error) {
	records, err := ports.ListProfiles(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}

	profiles := make([]account.Profile, 0, len(records))
	for _, record := range records {
		profiles = append(profiles, record.Profile)
	}

	return profiles, nil
}

// ViewerInputs reads the account and, for a non-empty profileID, the profile with what the
// effective access is built from. A profile of another account reads as unknown, so a session
// can never be resolved with a foreign profile's access.
func (ports accountPorts) ViewerInputs(ctx context.Context, accountID, profileID string) (appauth.ViewerInputs, error) {
	if profileID == "" {
		record, err := ports.GetAccount(ctx, accountID)
		if err != nil {
			return appauth.ViewerInputs{}, fmt.Errorf("viewer inputs of account %s: %w", accountID, err)
		}

		return appauth.ViewerInputs{
			Account: record.Account, AccountLibraries: record.Libraries, PrimaryLimit: record.PrimaryLimit,
		}, nil
	}

	inputs, err := ports.AccessInputs(ctx, profileID)
	if err != nil {
		return appauth.ViewerInputs{}, fmt.Errorf("viewer inputs of profile %s: %w", profileID, err)
	}

	if inputs.Account.ID != accountID {
		return appauth.ViewerInputs{}, fmt.Errorf("profile %s of another account: %w", profileID, domain.ErrNotFound)
	}

	return appauth.ViewerInputs{
		Account: inputs.Account, Profile: inputs.Profile, AccountLibraries: inputs.AccountLibraries,
		ProfileLibraries: inputs.ProfileLibraries, ProfileLimit: inputs.ProfileLimit, PrimaryLimit: inputs.PrimaryLimit,
	}, nil
}

// prepareAccounts builds the account services and runs the startup backfills. They finish
// before the listener exists, so no request sees a half-computed age bucket or a profile
// without its name key.
func (app application) prepareAccounts(ctx context.Context, db *sqlite.DB) (accountServices, error) {
	accounts, err := newAccountServices(db, app.logger)
	if err != nil {
		return accountServices{}, err
	}

	if err := app.runStartupBackfills(ctx, db, accounts); err != nil {
		return accountServices{}, err
	}

	return accounts, nil
}

// runStartupBackfills completes what migrations cannot compute in SQL, before the listener
// accepts requests: the age buckets of the catalog (ADR 0032 §7) and the name keys of
// profiles written before the keys existed. Both are idempotent.
func (app application) runStartupBackfills(ctx context.Context, db *sqlite.DB, accounts accountServices) error {
	if err := sqlite.NewLibraryCatalog(db, time.Now).BackfillAgeRatings(ctx); err != nil {
		return fmt.Errorf("backfill age ratings: %w", err)
	}

	unresolved, err := accounts.store.BackfillProfileNameKeys(ctx)
	if err != nil {
		return fmt.Errorf("backfill profile name keys: %w", err)
	}

	if unresolved > 0 {
		app.logger.WarnContext(ctx, "profiles share a name within their account and stay without a name key until renamed",
			slog.Int("profiles", unresolved))
	}

	return nil
}

// sessionPurger deletes expired sessions once at start and then every interval until its
// context ends. A failed purge is logged and retried at the next tick.
type sessionPurger struct {
	purge    func(ctx context.Context) (int64, error)
	interval time.Duration
	logger   *slog.Logger
}

// Run purges until ctx is done.
func (purger sessionPurger) Run(ctx context.Context) {
	ticker := time.NewTicker(purger.interval)
	defer ticker.Stop()

	for {
		purger.purgeOnce(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (purger sessionPurger) purgeOnce(ctx context.Context) {
	deleted, err := purger.purge(ctx)
	if err != nil {
		if ctx.Err() == nil {
			purger.logger.WarnContext(ctx, "purge expired sessions failed", slog.Any("error", err))
		}

		return
	}

	if deleted > 0 {
		purger.logger.DebugContext(ctx, "expired sessions purged", slog.Int64("sessions", deleted))
	}
}
