package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
)

// inWriteTransaction runs work in one transaction on the writer pool, which starts it with
// BEGIN IMMEDIATE (see dataSourceName). It commits when work succeeds and rolls back
// otherwise.
func inWriteTransaction(ctx context.Context, writer *sql.DB, work func(*sqlcgen.Queries) error) error {
	transaction, err := writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin write transaction: %w", err)
	}

	if err := work(sqlcgen.New(transaction)); err != nil {
		return errors.Join(err, rollBack(transaction))
	}

	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit write transaction: %w", err)
	}

	return nil
}

// rollBack ignores sql.ErrTxDone: database/sql has already rolled back a transaction whose
// context was cancelled.
func rollBack(transaction *sql.Tx) error {
	err := transaction.Rollback()
	if err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("roll back write transaction: %w", err)
	}

	return nil
}

// inReadTransaction runs work in one transaction on the reader pool, so its queries share a
// snapshot. The connections are query_only and nothing is written, so ending with a
// rollback is equivalent to a commit for success and failure alike.
func inReadTransaction(ctx context.Context, reader *sql.DB, work func(*sqlcgen.Queries) error) error {
	transaction, err := reader.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin read transaction: %w", err)
	}

	return errors.Join(work(sqlcgen.New(transaction)), endReadTransaction(transaction))
}

// endReadTransaction ignores sql.ErrTxDone, like rollBack.
func endReadTransaction(transaction *sql.Tx) error {
	err := transaction.Rollback()
	if err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("end read transaction: %w", err)
	}

	return nil
}
