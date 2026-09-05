package api

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/db"
)

// httpError is what handlers return to the strict-server machinery for
// expected failures; responseErrorHandler renders it as the envelope.
type httpError struct {
	status  int
	code    string
	message string
}

func (e *httpError) Error() string { return e.code + ": " + e.message }

func fail(status int, code, message string) error { return &httpError{status, code, message} }

// classify maps the auth package's sentinels and common database errors to
// envelopes; anything else stays an internal error (logged, masked).
func classify(err error) error {
	var he *httpError
	if errors.As(err, &he) {
		return err
	}
	var policy *auth.PasswordPolicyError
	switch {
	case errors.As(err, &policy):
		return fail(http.StatusBadRequest, "VALIDATION_FAILED", "Password "+policy.Requirement)
	case errors.Is(err, auth.ErrInvalidPassword):
		return fail(http.StatusUnauthorized, "INVALID_PASSWORD", "Password is incorrect")
	case errors.Is(err, auth.ErrEmailTaken):
		return fail(http.StatusConflict, "EMAIL_TAKEN", "An account already exists for that email")
	case errors.Is(err, auth.ErrSlugTaken):
		return fail(http.StatusConflict, "SLUG_TAKEN", "That slug is already in use")
	case errors.Is(err, auth.ErrNotMember), errors.Is(err, auth.ErrForbidden):
		return fail(http.StatusForbidden, "FORBIDDEN", "Access denied")
	case errors.Is(err, auth.ErrAlreadyMember):
		return fail(http.StatusConflict, "ALREADY_MEMBER", "Already a member of this organization")
	case errors.Is(err, auth.ErrInvitationExists):
		return fail(http.StatusConflict, "INVITATION_EXISTS", "A pending invitation already exists for that email")
	case errors.Is(err, auth.ErrInvitationEmailMismatch):
		return fail(http.StatusForbidden, "INVITATION_EMAIL_MISMATCH", "This invitation was sent to a different email address")
	case errors.Is(err, auth.ErrInvitationInvalid):
		return fail(http.StatusGone, "INVITATION_INVALID", "This invitation is no longer valid")
	case errors.Is(err, auth.ErrUnknownRole):
		return fail(http.StatusBadRequest, "VALIDATION_FAILED", "Unknown role")
	case errors.Is(err, auth.ErrNotFound), errors.Is(err, auth.ErrSessionNotFound), errors.Is(err, pgx.ErrNoRows):
		return fail(http.StatusNotFound, "NOT_FOUND", "Not found")
	case db.IsUniqueViolation(err):
		return fail(http.StatusConflict, "CONFLICT", "Already exists")
	}
	return err
}

// envelope renders an httpError through gen's error type so strict handlers
// can return typed 4xx bodies where the spec declares them.
func envelope(e *httpError) gen.Error {
	return gen.Error{Code: e.code, Message: e.message}
}
