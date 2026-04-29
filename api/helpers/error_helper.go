package helpers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
)

func ErrorResponse(err error) gin.H {
	log.Err(err).Msg("Error occurred")
	return gin.H{
		"message": err.Error(),
	}
}

// AbortIfReferenced detects a Postgres foreign-key violation (SQLSTATE 23503)
// and, if found, aborts the request with 409 Conflict plus a stable error code
// the frontend can localize. Returns true when the error was handled, so the
// caller can early-return instead of falling through to a generic 500.
func AbortIfReferenced(c *gin.Context, err error, code, message string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.ForeignKeyViolation {
		return false
	}
	c.AbortWithStatusJSON(http.StatusConflict, gin.H{
		"message": message,
		"code":    code,
	})
	return true
}

// AbortIfDuplicate detects a Postgres unique-key violation (SQLSTATE 23505)
// and, if found, aborts the request with 409 Conflict, surfacing the offending
// constraint so the frontend can show a specific message ("subdomain already
// taken", etc.). Postgres' Detail string is forwarded as the human message
// (e.g. "Key (subdomain)=(demo) already exists.") and the constraint name is
// returned as a stable code for i18n. Returns true when the error was handled.
func AbortIfDuplicate(c *gin.Context, err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.UniqueViolation {
		return false
	}
	message := pgErr.Detail
	if message == "" {
		message = pgErr.Message
	}
	c.AbortWithStatusJSON(http.StatusConflict, gin.H{
		"message":    message,
		"code":       "duplicate",
		"constraint": pgErr.ConstraintName,
	})
	return true
}

func ErrorStringResponse(errMsg string) gin.H {
	return gin.H{
		"message": errMsg,
	}
}
