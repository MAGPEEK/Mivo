package auth

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// This file extends fakeStore with the selection and device ports of M7-16.

func (store *fakeStore) ProfilePINHash(_ context.Context, profileID string) (string, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	for _, profiles := range store.profiles {
		for _, profile := range profiles {
			if profile.ID == profileID {
				return store.pins[profileID], nil
			}
		}
	}

	return "", fmt.Errorf("profile %s: %w", profileID, domain.ErrNotFound)
}

func (store *fakeStore) SetSessionProfile(_ context.Context, sessionID, profileID string, remember bool) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.failSelect != nil {
		return store.failSelect
	}

	for tokenHash, session := range store.sessions {
		if session.ID == sessionID {
			session.ProfileID, session.Remember = profileID, remember
			store.sessions[tokenHash] = session

			return nil
		}
	}

	return fmt.Errorf("session %s: %w", sessionID, domain.ErrNotFound)
}

func (store *fakeStore) AccountSessions(_ context.Context, accountID string) ([]Session, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.failDevices != nil {
		return nil, store.failDevices
	}

	var sessions []Session

	for _, session := range store.sessions {
		if session.AccountID == accountID {
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

func (store *fakeStore) AllSessions(context.Context) ([]AccountSession, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.failDevices != nil {
		return nil, store.failDevices
	}

	sessions := make([]AccountSession, 0, len(store.sessions))
	for _, session := range store.sessions {
		name := store.credentials[session.AccountID].Account.Name
		sessions = append(sessions, AccountSession{Session: session, AccountName: name})
	}

	return sessions, nil
}

func (store *fakeStore) SessionByID(_ context.Context, sessionID string) (Session, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if session, ok := store.sessionByIDLocked(sessionID); ok {
		return session, nil
	}

	return Session{}, fmt.Errorf("session %s: %w", sessionID, domain.ErrNotFound)
}

// setPIN stores a PIN hash for the profile and marks it HasPIN.
func (store *fakeStore) setPIN(profileID, pin string) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.pins[profileID] = hashPrefix + pin

	for _, profiles := range store.profiles {
		for index := range profiles {
			if profiles[index].ID == profileID {
				profiles[index].HasPIN = true
			}
		}
	}
}

func (store *fakeStore) session(sessionID string) (Session, bool) {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	return store.sessionByIDLocked(sessionID)
}

func (store *fakeStore) sessionByIDLocked(sessionID string) (Session, bool) {
	for _, session := range store.sessions {
		if session.ID == sessionID {
			return session, true
		}
	}

	return Session{}, false
}
