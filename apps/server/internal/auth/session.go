package auth

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/thecodearcher/limen"

	"github.com/refsdal/snarvei/server/internal/db/gen"
)

// SessionFromRequest validates the cookie or bearer token through Limen,
// then loads everything the app branches on from our own tables in one
// query. (nil, nil) means nobody is signed in. When Limen extended the
// session while validating it, the refreshed cookie is written to w.
func (s *service) SessionFromRequest(w http.ResponseWriter, r *http.Request) (*Session, error) {
	validated, err := s.limen.GetSession(r)
	if err != nil {
		if isSignedOut(err) {
			return nil, nil
		}
		return nil, wrap("validate session", err)
	}
	if validated == nil || validated.User == nil || validated.Session == nil {
		return nil, nil
	}
	row, err := s.q.GetAuthSession(r.Context(), gen.GetAuthSessionParams{
		Token:  validated.Session.Token,
		UserID: idString(validated.User.ID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, wrap("load session", err)
	}
	if validated.Refreshed != nil && w != nil {
		if err := s.core.Cookies().SetSessionCookie(w, validated.Refreshed); err != nil {
			return nil, wrap("refresh cookie", err)
		}
	}
	return &Session{
		UserID:               row.UserID,
		Name:                 row.Name,
		Email:                row.Email,
		Image:                row.Image,
		TwoFactorEnabled:     row.TwoFactorEnabled,
		SessionID:            row.SessionID,
		Token:                validated.Session.Token,
		ExpiresAt:            row.ExpiresAt.Time,
		ActiveOrganizationID: row.ActiveOrganizationID,
	}, nil
}

func isSignedOut(err error) bool {
	return errors.Is(err, limen.ErrSessionNotFound) || errors.Is(err, limen.ErrSessionExpired) ||
		errors.Is(err, limen.ErrSessionInvalid) || errors.Is(err, limen.ErrRecordNotFound)
}
