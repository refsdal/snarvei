package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/db"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/email"
)

const emailChangeTTL = time.Hour

func ts(t pgtype.Timestamptz) time.Time { return t.Time }

func (d Deps) me(ctx context.Context, s *auth.Session) (gen.Me, error) {
	row, err := d.Q.GetUserProfile(ctx, s.UserID)
	if err != nil {
		return gen.Me{}, err
	}
	var active *string
	if s.ActiveOrganizationID != "" {
		id := s.ActiveOrganizationID
		active = &id
	}
	return gen.Me{
		User:    gen.User{Id: row.ID, Name: row.Name, Email: row.Email, Image: row.Image, TwoFactorEnabled: row.TwoFactorEnabled},
		Session: gen.SessionInfo{Id: s.SessionID, ExpiresAt: s.ExpiresAt, ActiveOrganizationId: active},
	}, nil
}

func (d Deps) GetMe(ctx context.Context, _ gen.GetMeRequestObject) (gen.GetMeResponseObject, error) {
	me, err := d.me(ctx, middleware.SessionFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return gen.GetMe200JSONResponse(me), nil
}

func (d Deps) UpdateMe(ctx context.Context, req gen.UpdateMeRequestObject) (gen.UpdateMeResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	name := strings.TrimSpace(req.Body.Name)
	if name == "" || len(name) > 120 {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "Name is required")
	}
	if err := d.Q.UpdateUserName(ctx, dbgen.UpdateUserNameParams{ID: s.UserID, Name: &name}); err != nil {
		return nil, err
	}
	me, err := d.me(ctx, s)
	if err != nil {
		return nil, err
	}
	return gen.UpdateMe200JSONResponse(me), nil
}

func (d Deps) DeleteMe(ctx context.Context, req gen.DeleteMeRequestObject) (gen.DeleteMeResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	if err := d.Auth.VerifyPassword(ctx, s.UserID, req.Body.Password); err != nil {
		return nil, err
	}
	sole, err := d.Q.ListOrganizationsWhereSoleOwner(ctx, s.UserID)
	if err != nil {
		return nil, err
	}
	if len(sole) > 0 {
		return nil, fail(http.StatusConflict, "LAST_OWNER", "Transfer ownership of "+sole[0].Name+" before deleting your account")
	}
	if err := d.Auth.DeleteUser(ctx, s.UserID); err != nil {
		return nil, err
	}
	return gen.DeleteMe204Response{}, nil
}

func (d Deps) RequestEmailChange(ctx context.Context, req gen.RequestEmailChangeRequestObject) (gen.RequestEmailChangeResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	newEmail := strings.ToLower(strings.TrimSpace(string(req.Body.NewEmail)))
	if newEmail == "" || !strings.Contains(newEmail, "@") {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "A valid email is required")
	}
	if err := d.Auth.VerifyPassword(ctx, s.UserID, req.Body.Password); err != nil {
		return nil, err
	}
	if n, err := d.Q.CountUsersByEmail(ctx, newEmail); err != nil {
		return nil, err
	} else if n > 0 {
		return nil, fail(http.StatusConflict, "EMAIL_TAKEN", "An account already exists for that email")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	tok := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(tok))
	if err := d.Q.DeleteEmailChangeRequestsForUser(ctx, s.UserID); err != nil {
		return nil, err
	}
	if err := d.Q.CreateEmailChangeRequest(ctx, dbgen.CreateEmailChangeRequestParams{
		ID: auth.NewID(), UserID: s.UserID, NewEmail: newEmail, TokenHash: hex.EncodeToString(sum[:]),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(emailChangeTTL), Valid: true},
	}); err != nil {
		return nil, err
	}
	link := strings.TrimRight(d.AppURL, "/") + "/app/settings?emailToken=" + tok
	if err := d.Email.Send(ctx, email.EmailChange(d.AppName, newEmail, link).To(newEmail)); err != nil {
		d.log().Warn("email change mail failed", "event", "email.send_failed", "to", newEmail, "error", err.Error())
	}
	return gen.RequestEmailChange202Response{}, nil
}

func (d Deps) ConfirmEmailChange(ctx context.Context, req gen.ConfirmEmailChangeRequestObject) (gen.ConfirmEmailChangeResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	sum := sha256.Sum256([]byte(strings.TrimSpace(req.Body.Token)))
	row, err := d.Q.GetEmailChangeRequest(ctx, hex.EncodeToString(sum[:]))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (row.UserID != s.UserID || ts(row.ExpiresAt).Before(time.Now()))) {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "This link is invalid or has expired")
	}
	if err != nil {
		return nil, err
	}
	if err := d.Q.UpdateUserEmail(ctx, dbgen.UpdateUserEmailParams{ID: s.UserID, Email: row.NewEmail}); err != nil {
		if db.IsUniqueViolation(err) {
			return nil, fail(http.StatusConflict, "EMAIL_TAKEN", "An account already exists for that email")
		}
		return nil, err
	}
	if err := d.Q.DeleteEmailChangeRequestsForUser(ctx, s.UserID); err != nil {
		return nil, err
	}
	me, err := d.me(ctx, s)
	if err != nil {
		return nil, err
	}
	return gen.ConfirmEmailChange200JSONResponse(me), nil
}

func (d Deps) ListMySessions(ctx context.Context, _ gen.ListMySessionsRequestObject) (gen.ListMySessionsResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	rows, err := d.Q.ListUserSessions(ctx, s.UserID)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListMySessions200JSONResponse, 0, len(rows))
	for _, r := range rows {
		var ua *string
		var meta map[string]any
		if r.Metadata != "" && json.Unmarshal([]byte(r.Metadata), &meta) == nil {
			if v, ok := meta["user_agent"].(string); ok && v != "" {
				ua = &v
			}
		}
		var last *time.Time
		if r.LastAccess.Valid {
			t := r.LastAccess.Time
			last = &t
		}
		out = append(out, gen.SessionSummary{Id: r.ID, CreatedAt: ts(r.CreatedAt), LastAccess: last, ExpiresAt: ts(r.ExpiresAt), UserAgent: ua, Current: r.Token == s.Token})
	}
	return out, nil
}

func (d Deps) RevokeMySession(ctx context.Context, req gen.RevokeMySessionRequestObject) (gen.RevokeMySessionResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	row, err := d.Q.GetUserSessionByID(ctx, dbgen.GetUserSessionByIDParams{ID: req.SessionId, UserID: s.UserID})
	if err != nil {
		return nil, err // pgx.ErrNoRows → 404
	}
	if err := d.Auth.RevokeSession(ctx, row.Token); err != nil {
		return nil, err
	}
	return gen.RevokeMySession204Response{}, nil
}

func (d Deps) RevokeOtherSessions(ctx context.Context, _ gen.RevokeOtherSessionsRequestObject) (gen.RevokeOtherSessionsResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	rows, err := d.Q.ListUserSessions(ctx, s.UserID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.Token != s.Token {
			if err := d.Auth.RevokeSession(ctx, r.Token); err != nil && !errors.Is(err, auth.ErrNotFound) {
				return nil, err
			}
		}
	}
	return gen.RevokeOtherSessions204Response{}, nil
}
