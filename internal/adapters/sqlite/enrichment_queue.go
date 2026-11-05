package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
)

// EnrichmentReason tells why an item was queued (M6-11).
type EnrichmentReason string

// The reasons the enrichment_queue table accepts.
const (
	EnrichmentReasonScan     EnrichmentReason = "scan"
	EnrichmentReasonRefresh  EnrichmentReason = "refresh"
	EnrichmentReasonIdentify EnrichmentReason = "identify"
	EnrichmentReasonPeriodic EnrichmentReason = "periodic"
)

func (reason EnrichmentReason) isValid() bool {
	switch reason {
	case EnrichmentReasonScan, EnrichmentReasonRefresh, EnrichmentReasonIdentify, EnrichmentReasonPeriodic:
		return true
	default:
		return false
	}
}

// EnrichmentJob is a claimed queue entry. Revision identifies the request the claim saw;
// MarkJobDone and MarkJobFailed use it to keep a request that came in while the job ran.
// LeaseUntil is the lease of this claim; it proves the claim still owns the job.
type EnrichmentJob struct {
	ItemID     string
	Reason     EnrichmentReason
	NotBefore  time.Time
	Attempts   int
	LastError  string
	Revision   int64
	LeaseUntil time.Time
}

// Enqueue queues an item for enrichment, due now. An item is queued once: a repeated
// request takes the newer reason and is due now as well, but keeps the attempts, so the
// backoff of the next failure keeps growing. The error wraps domain.ErrInvalid for an unknown
// reason and domain.ErrNotFound for an unknown item.
func (store *EnrichmentStore) Enqueue(ctx context.Context, itemID string, reason EnrichmentReason) error {
	if !reason.isValid() {
		return fmt.Errorf("enqueue item %q: unknown reason %q: %w", itemID, reason, domain.ErrInvalid)
	}

	now := store.clock().UnixMilli()

	err := sqlcgen.New(store.writer).EnqueueEnrichment(ctx, sqlcgen.EnqueueEnrichmentParams{
		ItemID: itemID, Reason: string(reason), NotBefore: now, EnqueuedAt: now,
	})
	if err != nil {
		return fmt.Errorf("enqueue item %q: %w", itemID, foreignKeyAsNotFound(err))
	}

	return nil
}

// ClaimNextJob leases the next due job for lease and reports false when none is due. A job
// whose worker died without marking it is claimable again once its lease ran out, also
// after the database was reopened.
func (store *EnrichmentStore) ClaimNextJob(ctx context.Context, lease time.Duration) (EnrichmentJob, bool, error) {
	if lease <= 0 {
		return EnrichmentJob{}, false, fmt.Errorf("claim enrichment job: lease %v: %w", lease, domain.ErrInvalid)
	}

	now := store.clock().UnixMilli()

	// sqlc names the two current-time parameters after the columns they are compared with.
	row, err := sqlcgen.New(store.writer).ClaimEnrichment(ctx, sqlcgen.ClaimEnrichmentParams{
		ClaimedUntil:   sql.NullInt64{Int64: now + lease.Milliseconds(), Valid: true},
		NotBefore:      now,
		ClaimedUntil_2: sql.NullInt64{Int64: now, Valid: true},
	})
	if errors.Is(err, sql.ErrNoRows) {
		return EnrichmentJob{}, false, nil
	}

	if err != nil {
		return EnrichmentJob{}, false, fmt.Errorf("claim enrichment job: %w", err)
	}

	return EnrichmentJob{
		ItemID:     row.ItemID,
		Reason:     EnrichmentReason(row.Reason),
		NotBefore:  time.UnixMilli(row.NotBefore).UTC(),
		Attempts:   int(row.Attempts),
		LastError:  row.LastError,
		Revision:   row.Revision,
		LeaseUntil: time.UnixMilli(row.ClaimedUntil.Int64).UTC(),
	}, true, nil
}

// MarkJobDone removes a finished job. When the item was queued again while the job ran,
// the newer request stays and becomes claimable right away. A job that is gone (its item
// was deleted) is done as well. The error wraps domain.ErrConflict when another worker
// claimed the job after this claim's lease ran out; the job is then left alone.
func (store *EnrichmentStore) MarkJobDone(ctx context.Context, job EnrichmentJob) error {
	lease := sql.NullInt64{Int64: job.LeaseUntil.UnixMilli(), Valid: true}

	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		deleted, err := queries.DeleteEnrichmentRevision(ctx, sqlcgen.DeleteEnrichmentRevisionParams{
			ItemID: job.ItemID, Revision: job.Revision, ClaimedUntil: lease,
		})
		if err != nil || deleted > 0 {
			return err
		}

		released, err := queries.ReleaseEnrichment(ctx, sqlcgen.ReleaseEnrichmentParams{
			ItemID: job.ItemID, ClaimedUntil: lease,
		})
		if err != nil || released > 0 {
			return err
		}

		return lostClaim(ctx, queries, job.ItemID, nil)
	})
	if err != nil {
		return fmt.Errorf("mark enrichment of item %q done: %w", job.ItemID, err)
	}

	return nil
}

// MarkJobFailed releases a failed job with one more attempt, message as its last error and
// notBefore as the next due time (the backoff the caller chose). A request that came in
// while the job ran keeps its earlier due time. The error wraps domain.ErrNotFound when the
// job is gone, for example because its item was deleted, and domain.ErrConflict when
// another worker claimed it after this claim's lease ran out.
func (store *EnrichmentStore) MarkJobFailed(ctx context.Context, job EnrichmentJob, message string, notBefore time.Time) error {
	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		changed, err := queries.FailEnrichment(ctx, sqlcgen.FailEnrichmentParams{
			LastError:    message,
			Revision:     job.Revision,
			NotBefore:    notBefore.UnixMilli(),
			ItemID:       job.ItemID,
			ClaimedUntil: sql.NullInt64{Int64: job.LeaseUntil.UnixMilli(), Valid: true},
		})
		if err != nil || changed > 0 {
			return err
		}

		return lostClaim(ctx, queries, job.ItemID, domain.ErrNotFound)
	})
	if err != nil {
		return fmt.Errorf("mark enrichment of item %q failed: %w", job.ItemID, err)
	}

	return nil
}

// lostClaim explains why a finishing statement matched no row: the job is gone (whenGone)
// or another worker owns it now (domain.ErrConflict).
func lostClaim(ctx context.Context, queries *sqlcgen.Queries, itemID string, whenGone error) error {
	count, err := queries.CountEnrichmentJobsOfItem(ctx, itemID)
	if err != nil {
		return err
	}

	if count == 0 {
		return whenGone
	}

	return fmt.Errorf("job is claimed by another worker: %w", domain.ErrConflict)
}

// QueueLength returns the number of queued jobs, claimed or not.
func (store *EnrichmentStore) QueueLength(ctx context.Context) (int, error) {
	count, err := sqlcgen.New(store.reader).CountEnrichmentJobs(ctx)
	if err != nil {
		return 0, fmt.Errorf("count enrichment jobs: %w", err)
	}

	return int(count), nil
}
