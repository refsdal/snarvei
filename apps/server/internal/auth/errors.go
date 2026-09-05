package auth

import (
	"errors"
	"fmt"
)

// Sentinels callers branch on. Handlers map them to HTTP codes; nothing
// outside this package ever sees a Limen error value.
var (
	ErrInvalidPassword         = errors.New("auth: invalid password")
	ErrEmailTaken              = errors.New("auth: email already in use")
	ErrSlugTaken               = errors.New("auth: organization slug already in use")
	ErrNotMember               = errors.New("auth: not a member of this organization")
	ErrAlreadyMember           = errors.New("auth: already a member of this organization")
	ErrInvitationExists        = errors.New("auth: a pending invitation already exists for this email")
	ErrInvitationEmailMismatch = errors.New("auth: invitation was sent to a different email address")
	ErrInvitationInvalid       = errors.New("auth: invitation is no longer valid")
	ErrForbidden               = errors.New("auth: forbidden")
	ErrNotFound                = errors.New("auth: not found")
	ErrSessionNotFound         = errors.New("auth: session not found")
	ErrUnknownRole             = errors.New("auth: unknown role")
)

// PasswordPolicyError says a password failed Limen's policy (min 8 chars,
// an uppercase letter, a digit). Handlers answer 400 with the requirement.
type PasswordPolicyError struct{ Requirement string }

func (e *PasswordPolicyError) Error() string { return "auth: password " + e.Requirement }

func wrap(op string, err error) error { return fmt.Errorf("auth: %s: %w", op, err) }
