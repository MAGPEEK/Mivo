package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

const (
	sessionsPath = "/api/v1/playback/sessions"
	// codeSessionLimitReached is the problem code of a full server (503 with Retry-After).
	codeSessionLimitReached = "session_limit_reached"
)

// createSession starts a session. A full server is retried after its Retry-After until ctx
// ends: the previous case's session may still be winding down.
func createSession(ctx context.Context, client *apiClient, request apigen.PlaybackRequest, poll pollSettings) (apigen.PlaybackSession, error) {
	for {
		var session apigen.PlaybackSession

		err := client.call(ctx, apiCall{
			method: http.MethodPost, reference: sessionsPath, body: request, want: http.StatusCreated, out: &session,
		})
		if err == nil {
			return session, nil
		}

		problem, isProblem := errors.AsType[*problemError](err)
		if !isProblem || problem.code != codeSessionLimitReached {
			return apigen.PlaybackSession{}, fmt.Errorf("create the session: %w", err)
		}

		if err := poll.wait(ctx, problem.retryAfter); err != nil {
			return apigen.PlaybackSession{}, fmt.Errorf("create the session: %w", err)
		}
	}
}

func getSession(ctx context.Context, client *apiClient, id uuid.UUID) (apigen.PlaybackSession, error) {
	var session apigen.PlaybackSession

	err := client.call(ctx, apiCall{
		method: http.MethodGet, reference: sessionsPath + "/" + id.String(), want: http.StatusOK, out: &session,
	})
	if err != nil {
		return apigen.PlaybackSession{}, fmt.Errorf("read the session: %w", err)
	}

	return session, nil
}

// stopSession deletes the session, which ends its FFmpeg and removes its files.
func stopSession(ctx context.Context, client *apiClient, id uuid.UUID) error {
	err := client.call(ctx, apiCall{
		method: http.MethodDelete, reference: sessionsPath + "/" + id.String(), want: http.StatusNoContent,
	})
	if err != nil {
		return fmt.Errorf("stop the session: %w", err)
	}

	return nil
}
