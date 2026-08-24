package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// uniqueViolationCode is the PostgreSQL SQLSTATE for a unique constraint
// violation.
const uniqueViolationCode = "23505"

// isUniqueViolation reports whether err is a unique-constraint violation,
// i.e. the case internal/domain.ErrDuplicateKey represents.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode
}
