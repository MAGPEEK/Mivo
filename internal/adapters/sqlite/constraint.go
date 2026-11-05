package sqlite

import (
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"

	sqlitedriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// removeBatchSize bounds the IDs bound to one statement of a bulk removal. SQLite accepts at
// most 32 766 parameters per statement; a vanished folder can hold more files than that.
const removeBatchSize = 500

// isUniqueViolation reports a UNIQUE constraint failure. The connections report extended
// result codes, so the code tells it apart from other constraint errors.
func isUniqueViolation(err error) bool {
	var driverError *sqlitedriver.Error

	return errors.As(err, &driverError) && driverError.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

// foreignKeyAsNotFound turns a FOREIGN KEY constraint failure into domain.ErrNotFound: the
// row the statement refers to does not exist. The driver detail is dropped so no schema
// text reaches the error message. Other errors pass through unchanged.
func foreignKeyAsNotFound(err error) error {
	var driverError *sqlitedriver.Error
	if errors.As(err, &driverError) && driverError.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY {
		return fmt.Errorf("referenced row does not exist: %w", domain.ErrNotFound)
	}

	return err
}
