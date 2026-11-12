package playback

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// SessionInfo is the operator view of one session (ADR 0030 section 7). It carries no paths.
type SessionInfo struct {
	ID string
	// FileID names the played item.
	FileID string
	State  State
	Mode   playback.Mode
	// Encoder is the name of the video encoder, empty when no video is re-encoded; Hardware
	// tells whether it is a hardware encoder.
	Encoder  string
	Hardware bool
	// OutputBitRate is the bit rate FFmpeg reports in bits per second, else the session's
	// planned rate; 0 when unknown.
	OutputBitRate int64
	// Speed is the production speed relative to playback, 0 when not reported.
	Speed float64
	// Paused is true while the throttle controller holds the production.
	Paused bool
	// ClientName is the name of the client profile that started the session.
	ClientName string
	// Local is true when the client was in the local network at the start; the zero value is
	// remote, the stricter reading.
	Local bool
	// Reasons are the plan's reasons why a cheaper mode was not possible; empty for direct
	// play. The slice belongs to the caller.
	Reasons []playback.Reason
	// CPUPercent is the CPU use of the FFmpeg process in percent of one core; 0 when unknown.
	CPUPercent float64
	StartedAt  time.Time
}

// listed is a registry snapshot of one session plus what List reads from its production
// outside the lock.
type listed struct {
	session    Session
	encoder    playback.Encoder
	clientName string
	local      bool
	paused     bool
	production Production
}

// List returns the visible sessions, oldest first: an administrator sees all, anyone else
// only the sessions of their own sign-in session (ADR 0024, Notes M7-14). Without a viewer the
// error wraps domain.ErrUnauthenticated.
func (s *Service) List(ctx context.Context) ([]SessionInfo, error) {
	viewer, ok := appaccess.ViewerFrom(ctx)
	if !ok {
		return nil, fmt.Errorf("viewer: %w", domain.ErrUnauthenticated)
	}

	snapshots := s.sessions.snapshot(func(owner string) bool { return mayReach(viewer, owner) })
	infos := make([]SessionInfo, 0, len(snapshots))

	for _, snapshot := range snapshots {
		infos = append(infos, infoOf(snapshot))
	}

	return infos, nil
}

func infoOf(snapshot listed) SessionInfo {
	session := snapshot.session
	info := SessionInfo{
		ID: session.ID, FileID: session.FileID, State: session.State, Mode: session.Plan.Mode,
		Encoder: snapshot.encoder.Name, Hardware: snapshot.encoder.Hardware,
		OutputBitRate: session.OutputBitRate, Paused: snapshot.paused,
		ClientName: snapshot.clientName, Local: snapshot.local, Reasons: session.Plan.Reasons,
		StartedAt: session.CreatedAt,
	}

	if reporter, ok := snapshot.production.(CPUReporter); ok {
		if percent, known := reporter.CPUPercent(); known {
			info.CPUPercent = percent
		}
	}

	if reporter, ok := snapshot.production.(ProgressReporter); ok {
		progress := reporter.Progress()
		info.Speed = progress.Speed

		if progress.BitRate > 0 {
			info.OutputBitRate = progress.BitRate
		}
	}

	return info
}

// snapshot copies the visible sessions whose owner passes allow, oldest first.
func (sessions *registry) snapshot(allow func(owner string) bool) []listed {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	var result []listed

	for _, entry := range sessions.entries {
		if entry.pending || !allow(entry.owner) {
			continue
		}

		result = append(result, listed{
			session: entry.session.clone(), encoder: entry.encoder, clientName: entry.clientName,
			local: entry.local, paused: entry.paused, production: entry.production,
		})
	}

	slices.SortFunc(result, func(a, b listed) int {
		return cmp.Or(a.session.CreatedAt.Compare(b.session.CreatedAt), cmp.Compare(a.session.ID, b.session.ID))
	})

	return result
}
