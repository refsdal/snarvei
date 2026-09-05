// Package auth is the ONLY package that imports Limen. Everything else
// consumes Service and Session. The instance is built once at startup.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/thecodearcher/limen"
	sqladapter "github.com/thecodearcher/limen/adapters/sql"
	credentialpassword "github.com/thecodearcher/limen/plugins/credential-password"
	"github.com/thecodearcher/limen/plugins/organization"
	twofactor "github.com/thecodearcher/limen/plugins/two-factor"

	"github.com/refsdal/snarvei/server/internal/authz"
	"github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/email"
)

// BasePath is where Limen's router is mounted; it must match the mount point exactly.
const BasePath = "/api/auth"

// SessionCookieName is pinned so an upgrade cannot rename it under us.
const SessionCookieName = "snarvei_session"

// Session is what the rest of the app knows about the caller.
type Session struct {
	UserID               string
	Name                 string
	Email                string
	Image                *string
	TwoFactorEnabled     bool
	SessionID            string
	Token                string
	ExpiresAt            time.Time
	ActiveOrganizationID string // "" when none
}

// Organization is the subset of Limen's organization the app uses.
type Organization struct{ ID, Name, Slug string }

// Invitation is the subset of Limen's invitation the app uses.
type Invitation struct {
	ID, OrganizationID, Email, Role, Status string
	ExpiresAt                               *time.Time
}

// Config is what the composition root supplies.
type Config struct {
	AppURL     string
	AppName    string
	Secret     string
	OpenSignup bool
	Pool       *pgxpool.Pool
	// ClientIP returns the keyed digest of the client address; Limen uses it
	// for session metadata and rate-limit keys so no raw address is stored.
	ClientIP func(*http.Request) string
	Email    email.Sender
	Log      *slog.Logger
}

// Service is the entire auth surface the rest of the app may use.
type Service interface {
	Handler() http.Handler
	SessionFromRequest(w http.ResponseWriter, r *http.Request) (*Session, error)
	CreateUser(ctx context.Context, name, email, password string) (string, error)
	VerifyPassword(ctx context.Context, userID, password string) error
	StartSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID string) error
	CreateOrganization(ctx context.Context, userID, name, slug string) (*Organization, error)
	SetActiveOrganization(ctx context.Context, sessionToken, orgID string) error
	CreateInvitation(ctx context.Context, inviterUserID, orgID, emailAddr, role string) (*Invitation, error)
	AcceptInvitation(ctx context.Context, userID, invitationID string) (*Invitation, error)
	RejectInvitation(ctx context.Context, userID, invitationID string) error
	CancelInvitation(ctx context.Context, actorUserID, orgID, invitationID string) error
	RevokeSession(ctx context.Context, token string) error
	RevokeAllSessions(ctx context.Context, userID string) error
	DeleteUser(ctx context.Context, userID string) error
}

type service struct {
	limen *limen.Limen
	core  *limen.LimenCore
	org   organization.API
	cred  credentialpassword.API
	pool  *pgxpool.Pool
	q     *gen.Queries
	cfg   Config
	log   *slog.Logger
}

var _ Service = (*service)(nil)

// New builds the Limen instance and wires it to Snarvei's schema.
func New(cfg Config) (Service, error) {
	switch {
	case cfg.Pool == nil:
		return nil, errors.New("auth: a database pool is required")
	case cfg.AppURL == "":
		return nil, errors.New("auth: AppURL is required")
	case len(cfg.Secret) < 32:
		return nil, errors.New("auth: Secret must be at least 32 bytes")
	case cfg.ClientIP == nil:
		return nil, errors.New("auth: ClientIP extractor is required")
	case cfg.Email == nil:
		return nil, errors.New("auth: Email sender is required")
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.AppName == "" {
		cfg.AppName = "Snarvei"
	}

	sqlDB := stdlib.OpenDBFromPool(cfg.Pool)
	secret := sha256.Sum256([]byte(cfg.Secret))
	core := &corePlugin{}
	appURL := strings.TrimRight(cfg.AppURL, "/")

	credPlugin := credentialpassword.New(
		credentialpassword.WithSendPasswordResetEmail(func(to, token string) {
			link := appURL + "/reset-password?token=" + token
			if err := cfg.Email.Send(context.Background(), email.PasswordReset(cfg.AppName, link).To(to)); err != nil {
				cfg.Log.Warn("password reset mail failed", "event", "email.send_failed", "to", to, "error", err.Error())
			}
		}),
		// ResetPassword (unlike SetPassword/UpdatePassword) takes no
		// revokeOtherSessions flag of its own — confirmed against the
		// pinned v0.2.0 source (password.go): it updates the password and
		// deletes the verification token, nothing else. Without this hook a
		// stolen session would survive its own victim's password reset.
		// core is captured by pointer; it is nil until limen.New below
		// initializes the core plugin, which always happens before any
		// HTTP request can reach this callback.
		credentialpassword.WithOnPasswordResetSuccess(func(ctx context.Context, user *limen.User) {
			if err := core.core.SessionManager.RevokeAllSessions(ctx, user.ID); err != nil {
				cfg.Log.Warn("revoke sessions after password reset failed", "event", "auth.revoke_failed", "error", err.Error())
			}
		}),
	)
	twoFactorPlugin := twofactor.New(
		twofactor.WithTOTP(twofactor.WithTOTPIssuer(cfg.AppName)),
		twofactor.WithRevokeOtherSessionsOnStateChange(true),
	)
	orgPlugin := organization.New(
		organization.WithSlugGenerator(func(_ string, provided string) string { return provided }),
		organization.WithSendInvitationMail(func(ctx context.Context, data *organization.SendInvitationMailData) {
			inviter := ""
			if data.Inviter != nil {
				if name, ok := data.Inviter.Raw()["name"].(string); ok {
					inviter = name
				}
			}
			link := appURL + "/app/invitations/" + idString(data.Invitation.ID)
			msg := email.Invitation(cfg.AppName, data.Organization.Name, inviter, link).To(data.Invitation.Email)
			if err := cfg.Email.Send(ctx, msg); err != nil {
				cfg.Log.Warn("invitation mail failed", "event", "email.send_failed", "to", data.Invitation.Email, "error", err.Error())
			}
		}),
	)

	instance, err := limen.New(&limen.Config{
		BaseURL:  appURL,
		Database: sqladapter.NewPostgreSQL(sqlDB),
		Secret:   secret[:],
		Schema: limen.NewDefaultSchemaConfig(
			limen.WithSchemaIDGenerator(uuidGenerator{}),
			// The sign-up route carries {name, email, password}; Limen only
			// knows email/password, so name is picked off the body here.
			limen.WithSchemaUser(limen.WithUserAdditionalFields(func(ctx *limen.AdditionalFieldsContext) (map[string]any, error) {
				if !ctx.IsCreate() {
					return nil, nil
				}
				fields := map[string]any{}
				if name, ok := ctx.GetBodyValue("name").(string); ok && strings.TrimSpace(name) != "" {
					fields["name"] = strings.TrimSpace(name)
				}
				return fields, nil
			})),
		),
		Session: limen.NewDefaultSessionConfig(
			limen.WithSessionIPAddressExtractor(cfg.ClientIP),
		),
		HTTP: limen.NewDefaultHTTPConfig(
			limen.WithHTTPBasePath(BasePath),
			limen.WithHTTPSessionCookieName(SessionCookieName),
			limen.WithHTTPCookieSecure(strings.HasPrefix(appURL, "https://")),
			limen.WithHTTPTrustedOrigins([]string{appURL}),
			limen.WithHTTPDisabledPaths(disabledRouteIDs(cfg.OpenSignup)),
			// StoreTypeDatabase is NOT used here: Limen v0.2.1's
			// RateLimitSchema.FromStorage (rate_limit.go) hard-casts the
			// stored count to int32, but pgx v5's stdlib driver widens every
			// Postgres integer OID (int2/int4/int8) to int64 to satisfy
			// database/sql's driver.Value contract — confirmed against the
			// pinned pgx v5.10.0 source (stdlib/sql.go's Int2OID/Int4OID
			// valueFuncs both `return int64(d), err`). That type assertion
			// panics on every rate-limited request through this adapter,
			// for any Postgres column type; no migration changes it. The
			// in-memory cache store (Limen's own default, and what Pjokk's
			// working integration uses) sidesteps the incompatible code path
			// while keeping the same intent: throttling the auth routes.
			limen.WithHTTPRateLimiter(
				limen.WithRateLimiterKeyGenerator(cfg.ClientIP),
				limen.WithRateLimiterMaxRequests(60),
				limen.WithRateLimiterWindow(time.Minute),
			),
		),
		Plugins: []limen.Plugin{credPlugin, twoFactorPlugin, orgPlugin, core},
	})
	if err != nil {
		return nil, fmt.Errorf("auth: build limen: %w", err)
	}
	if core.core == nil {
		return nil, errors.New("auth: limen did not initialize the core plugin")
	}
	svc := &service{
		limen: instance,
		core:  core.core,
		org:   organization.Use(instance),
		cred:  credentialpassword.Use(instance),
		pool:  cfg.Pool,
		q:     gen.New(cfg.Pool),
		cfg:   cfg,
		log:   cfg.Log,
	}
	return svc, nil
}

func (s *service) Handler() http.Handler { return s.limen.Handler() }

func (s *service) CreateUser(ctx context.Context, name, emailAddr, password string) (string, error) {
	emailAddr = limen.NormalizeEmail(emailAddr)
	result, err := s.cred.SignUpWithCredentialAndPassword(ctx, &limen.User{Email: emailAddr, Password: &password}, map[string]any{"name": strings.TrimSpace(name)})
	if err != nil {
		return "", mapError("create user", err)
	}
	return idString(result.User.ID), nil
}

func (s *service) VerifyPassword(ctx context.Context, userID, password string) error {
	user, err := s.core.DBAction.FindUserByID(ctx, userID)
	if err != nil {
		return mapError("load user", err)
	}
	ok, err := s.cred.ComparePassword(password, user.Password)
	if err != nil {
		return wrap("compare password", err)
	}
	if !ok {
		return ErrInvalidPassword
	}
	return nil
}

func (s *service) StartSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID string) error {
	user, err := s.core.DBAction.FindUserByID(ctx, userID)
	if err != nil {
		return mapError("load user", err)
	}
	result, err := s.core.CreateSession(ctx, r, w, &limen.AuthenticationResult{User: user})
	if err != nil {
		return wrap("create session", err)
	}
	if err := s.core.Cookies().SetSessionCookie(w, result); err != nil {
		return wrap("set session cookie", err)
	}
	return nil
}

func (s *service) CreateOrganization(ctx context.Context, userID, name, slug string) (*Organization, error) {
	user, err := s.core.DBAction.FindUserByID(ctx, userID)
	if err != nil {
		return nil, mapError("load user", err)
	}
	org, err := s.org.CreateOrganization(ctx, user, &organization.CreateOrganizationRequest{Name: name, Slug: slug})
	if err != nil {
		return nil, mapError("create organization", err)
	}
	return &Organization{ID: idString(org.ID), Name: org.Name, Slug: org.Slug}, nil
}

// SetActiveOrganization checks membership first: the middleware trusts the
// session's active organization, so writing it unchecked would let any user
// read any organization by naming its id.
func (s *service) SetActiveOrganization(ctx context.Context, sessionToken, orgID string) error {
	record, err := s.q.GetSessionRecord(ctx, sessionToken)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionNotFound
		}
		return wrap("load session", err)
	}
	n, err := s.q.CountOrganizationMembership(ctx, gen.CountOrganizationMembershipParams{OrganizationID: orgID, UserID: record.UserID})
	if err != nil {
		return wrap("check membership", err)
	}
	if n == 0 {
		return ErrNotMember
	}
	if err := s.q.SetSessionActiveOrganization(ctx, gen.SetSessionActiveOrganizationParams{Token: sessionToken, ActiveOrganizationID: &orgID}); err != nil {
		return wrap("set active organization", err)
	}
	return nil
}

func (s *service) CreateInvitation(ctx context.Context, inviterUserID, orgID, emailAddr, role string) (*Invitation, error) {
	if !authz.IsValidInviteRole(role) {
		return nil, ErrUnknownRole
	}
	inviter, err := s.core.DBAction.FindUserByID(ctx, inviterUserID)
	if err != nil {
		return nil, mapError("load inviter", err)
	}
	org, err := s.org.GetOrganization(ctx, orgID)
	if err != nil {
		return nil, mapError("load organization", err)
	}
	inv, err := s.org.CreateInvitation(ctx, inviter, org, &organization.CreateInvitationRequest{Email: emailAddr, Role: role})
	if err != nil {
		return nil, mapError("create invitation", err)
	}
	return toInvitation(inv), nil
}

func (s *service) respond(ctx context.Context, userID, invitationID string, response organization.InvitationResponse) (*Invitation, error) {
	user, err := s.core.DBAction.FindUserByID(ctx, userID)
	if err != nil {
		return nil, mapError("load user", err)
	}
	row, err := s.q.GetInvitationToken(ctx, invitationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, wrap("load invitation", err)
	}
	if row.Status != string(organization.InvitationStatusPending) {
		return nil, ErrInvitationInvalid
	}
	inv, err := s.org.RespondToInvitation(ctx, user, row.Token, response)
	if err != nil {
		return nil, mapError("respond to invitation", err)
	}
	return toInvitation(inv), nil
}

func (s *service) AcceptInvitation(ctx context.Context, userID, invitationID string) (*Invitation, error) {
	return s.respond(ctx, userID, invitationID, organization.InvitationResponseAccept)
}

func (s *service) RejectInvitation(ctx context.Context, userID, invitationID string) error {
	_, err := s.respond(ctx, userID, invitationID, organization.InvitationResponseReject)
	return err
}

func (s *service) CancelInvitation(ctx context.Context, actorUserID, orgID, invitationID string) error {
	actor, err := s.core.DBAction.FindUserByID(ctx, actorUserID)
	if err != nil {
		return mapError("load user", err)
	}
	org, err := s.org.GetOrganization(ctx, orgID)
	if err != nil {
		return mapError("load organization", err)
	}
	if _, err := s.org.CancelPendingInvitation(ctx, actor, org, invitationID); err != nil {
		return mapError("cancel invitation", err)
	}
	return nil
}

func (s *service) RevokeSession(ctx context.Context, token string) error {
	if err := s.limen.RevokeSession(ctx, token); err != nil {
		return mapError("revoke session", err)
	}
	return nil
}

func (s *service) RevokeAllSessions(ctx context.Context, userID string) error {
	if err := s.limen.RevokeAllSessions(ctx, userID); err != nil {
		return wrap("revoke sessions", err)
	}
	return nil
}

func (s *service) DeleteUser(ctx context.Context, userID string) error {
	if err := s.RevokeAllSessions(ctx, userID); err != nil {
		return err
	}
	if err := s.q.DeleteUser(ctx, userID); err != nil {
		return wrap("delete user", err)
	}
	return nil
}

// --- helpers ---------------------------------------------------------

func toInvitation(inv *organization.Invitation) *Invitation {
	role := ""
	if len(inv.Roles) > 0 {
		role = idString(inv.Roles[0])
	}
	return &Invitation{
		ID: idString(inv.ID), OrganizationID: idString(inv.OrganizationID), Email: inv.Email,
		Role: role, Status: string(inv.Status), ExpiresAt: inv.ExpiresAt,
	}
}

// mapError translates Limen's errors into this package's sentinels so no
// Limen type crosses the package boundary.
func mapError(op string, err error) error {
	switch {
	case errors.Is(err, credentialpassword.ErrEmailAlreadyExists):
		return ErrEmailTaken
	case errors.Is(err, credentialpassword.ErrInvalidCredential), errors.Is(err, credentialpassword.ErrInvalidPassword), errors.Is(err, credentialpassword.ErrInvalidCurrentPassword):
		return ErrInvalidPassword
	case errors.Is(err, credentialpassword.ErrPasswordTooShort):
		return &PasswordPolicyError{Requirement: "must be at least 8 characters"}
	case errors.Is(err, credentialpassword.ErrPasswordRequiresUppercase):
		return &PasswordPolicyError{Requirement: "must contain an uppercase letter"}
	case errors.Is(err, credentialpassword.ErrPasswordRequiresNumbers):
		return &PasswordPolicyError{Requirement: "must contain a number"}
	case errors.Is(err, organization.ErrOrganizationSlugAlreadyExists), errors.Is(err, organization.ErrInvalidSlug):
		return ErrSlugTaken
	case errors.Is(err, organization.ErrUserAlreadyInOrganization), errors.Is(err, organization.ErrMemberAlreadyExists):
		return ErrAlreadyMember
	case errors.Is(err, organization.ErrInvitationAlreadyExists):
		return ErrInvitationExists
	case errors.Is(err, organization.ErrInvitationEmailMismatch):
		return ErrInvitationEmailMismatch
	case errors.Is(err, organization.ErrInvalidInvitation):
		return ErrInvitationInvalid
	case errors.Is(err, organization.ErrInsufficientPermission), errors.Is(err, organization.ErrUserCannotInviteOwner), errors.Is(err, organization.ErrMemberNotInOrganization):
		return ErrForbidden
	case errors.Is(err, limen.ErrRecordNotFound):
		return ErrNotFound
	}
	return wrap(op, err)
}

type uuidGenerator struct{}

func (uuidGenerator) Generate(context.Context) (any, error) { return NewID(), nil }
func (uuidGenerator) GetColumnType() limen.ColumnType       { return limen.ColumnTypeString }

// NewID returns a random UUIDv4 string; the same generator Limen uses for
// its tables, exported for Snarvei's own inserts.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("auth: crypto/rand failed: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func idString(id any) string {
	if s, ok := id.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", id)
}

// RolesFromJSON decodes Limen's roles column (a JSON array in text) — used by
// handlers that read organization_invitations.roles through sqlc.
func RolesFromJSON(raw string) []string {
	var out []string
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{raw}
	}
	return out
}
