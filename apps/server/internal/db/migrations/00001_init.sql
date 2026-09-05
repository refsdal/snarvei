-- +goose Up

-- ===========================================================================
-- Auth tables (Limen-shaped). Column sets come from Pjokk's runtime-verified
-- migrations (00001_init + 00002_limen_align) and, for two_factors, from the
-- plugin's own schema definition. Limen does not migrate or validate the
-- schema itself; a mismatch here surfaces as a SQL error on first use.
-- ===========================================================================

CREATE TABLE "users" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"public_id" text NOT NULL DEFAULT gen_random_uuid()::text,
	"first_name" text,
	"last_name" text,
	"email" text NOT NULL,
	"password" text,
	"email_verified_at" timestamptz,
	"two_factor_enabled" boolean NOT NULL DEFAULT false,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now(),
	"deleted_at" timestamptz,
	-- Snarvei's own fields, supplied through Limen's additional-fields map.
	"name" text,
	"image" text,
	CONSTRAINT "users_email_unique" UNIQUE ("email"),
	CONSTRAINT "users_public_id_unique" UNIQUE ("public_id")
);

CREATE TABLE "organizations" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"name" text NOT NULL,
	"slug" text NOT NULL,
	"logo" text,
	"metadata" text,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT "organizations_slug_unique" UNIQUE ("slug")
);

CREATE TABLE "sessions" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"user_id" text NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
	"token" text NOT NULL,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"expires_at" timestamptz NOT NULL,
	"last_access" timestamptz,
	"metadata" text,
	"active_organization_id" text REFERENCES "organizations" ("id") ON DELETE SET NULL,
	CONSTRAINT "sessions_token_unique" UNIQUE ("token")
);
CREATE INDEX "sessions_user_idx" ON "sessions" ("user_id");
CREATE INDEX "idx_sessions_active_organization" ON "sessions" ("active_organization_id");

CREATE TABLE "accounts" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"user_id" text NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
	"provider" text NOT NULL,
	"provider_account_id" text NOT NULL,
	"access_token" text,
	"refresh_token" text,
	"id_token" text,
	"access_token_expires_at" timestamptz,
	"refresh_token_expires_at" timestamptz,
	"scope" text,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT "accounts_provider_account_unique" UNIQUE ("provider", "provider_account_id")
);
CREATE INDEX "accounts_user_idx" ON "accounts" ("user_id");

CREATE TABLE "verifications" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"subject" text NOT NULL,
	"value" text NOT NULL,
	"expires_at" timestamptz NOT NULL,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX "idx_verifications_subject" ON "verifications" ("subject");
CREATE UNIQUE INDEX "idx_verifications_value" ON "verifications" ("value");

-- Limen's own rate limiter table (distinct from Snarvei's rate_limit below).
CREATE TABLE "rate_limits" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"key" text NOT NULL,
	"count" integer NOT NULL DEFAULT 0,
	"last_request_at" bigint NOT NULL DEFAULT 0,
	CONSTRAINT "rate_limits_key_unique" UNIQUE ("key")
);

CREATE TABLE "two_factors" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"user_id" text NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
	"secret" text,
	"backup_codes" text
);
CREATE UNIQUE INDEX "idx_two_factors_user_id" ON "two_factors" ("user_id");

CREATE TABLE "organization_members" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"organization_id" text NOT NULL REFERENCES "organizations" ("id") ON DELETE CASCADE,
	"user_id" text NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX "organization_members_org_idx" ON "organization_members" ("organization_id");
CREATE INDEX "organization_members_user_idx" ON "organization_members" ("user_id");
CREATE UNIQUE INDEX "idx_organization_members_org_user" ON "organization_members" ("organization_id", "user_id");

CREATE TABLE "organization_member_roles" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"member_id" text NOT NULL REFERENCES "organization_members" ("id") ON DELETE CASCADE,
	"organization_id" text NOT NULL REFERENCES "organizations" ("id") ON DELETE CASCADE,
	"role" text,
	"created_at" timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX "idx_organization_member_roles_member_role" ON "organization_member_roles" ("member_id", "role");
CREATE INDEX "organization_member_roles_org_idx" ON "organization_member_roles" ("organization_id");

CREATE TABLE "organization_invitations" (
	"id" text PRIMARY KEY DEFAULT gen_random_uuid()::text,
	"organization_id" text NOT NULL REFERENCES "organizations" ("id") ON DELETE CASCADE,
	"email" text NOT NULL,
	"roles" text,
	"status" text NOT NULL DEFAULT 'pending',
	"token" text NOT NULL,
	"expires_at" timestamptz,
	"inviter_id" text REFERENCES "users" ("id") ON DELETE CASCADE,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX "organization_invitations_org_idx" ON "organization_invitations" ("organization_id");
CREATE INDEX "organization_invitations_email_idx" ON "organization_invitations" ("email");
CREATE UNIQUE INDEX "idx_organization_invitations_token" ON "organization_invitations" ("token");

-- ===========================================================================
-- Snarvei tables (spec section 4)
-- ===========================================================================

CREATE TABLE "teams" (
	"id" text PRIMARY KEY,
	"organization_id" text NOT NULL REFERENCES "organizations" ("id") ON DELETE CASCADE,
	"name" text NOT NULL,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT "teams_org_name_unique" UNIQUE ("organization_id", "name")
);
CREATE INDEX "teams_org_idx" ON "teams" ("organization_id");

CREATE TABLE "team_members" (
	"team_id" text NOT NULL REFERENCES "teams" ("id") ON DELETE CASCADE,
	"user_id" text NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT "team_members_pk" PRIMARY KEY ("team_id", "user_id")
);
CREATE INDEX "team_members_user_idx" ON "team_members" ("user_id");

CREATE TABLE "invitation_teams" (
	"invitation_id" text PRIMARY KEY REFERENCES "organization_invitations" ("id") ON DELETE CASCADE,
	"team_id" text NOT NULL REFERENCES "teams" ("id") ON DELETE CASCADE
);

CREATE TABLE "email_change_requests" (
	"id" text PRIMARY KEY,
	"user_id" text NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
	"new_email" text NOT NULL,
	"token_hash" text NOT NULL,
	"expires_at" timestamptz NOT NULL,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT "email_change_requests_token_unique" UNIQUE ("token_hash")
);
CREATE INDEX "email_change_requests_user_idx" ON "email_change_requests" ("user_id");

CREATE TABLE "links" (
	"id" text PRIMARY KEY,
	"organization_id" text NOT NULL REFERENCES "organizations" ("id") ON DELETE CASCADE,
	"team_id" text NOT NULL REFERENCES "teams" ("id") ON DELETE CASCADE,
	"slug" text NOT NULL,
	"target_url" text NOT NULL,
	"redirect_status" smallint NOT NULL DEFAULT 302 CHECK ("redirect_status" IN (301, 302, 307)),
	"is_active" boolean NOT NULL DEFAULT true,
	"title" text,
	"description" text,
	-- Authorship is informational: links belong to the team, so deleting a
	-- user must never delete links.
	"created_by" text REFERENCES "users" ("id") ON DELETE SET NULL,
	"updated_by" text REFERENCES "users" ("id") ON DELETE SET NULL,
	"created_at" timestamptz NOT NULL DEFAULT now(),
	"updated_at" timestamptz NOT NULL DEFAULT now(),
	CONSTRAINT "links_slug_unique" UNIQUE ("slug")
);
CREATE INDEX "links_team_idx" ON "links" ("team_id");
CREATE INDEX "links_org_idx" ON "links" ("organization_id");

CREATE TABLE "link_target_history" (
	"id" text PRIMARY KEY,
	"link_id" text NOT NULL REFERENCES "links" ("id") ON DELETE CASCADE,
	"old_target_url" text,
	"new_target_url" text NOT NULL,
	"changed_by" text REFERENCES "users" ("id") ON DELETE SET NULL,
	"changed_at" timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX "link_target_history_link_idx" ON "link_target_history" ("link_id");

CREATE TABLE "click_events" (
	"id" text PRIMARY KEY,
	"link_id" text NOT NULL REFERENCES "links" ("id") ON DELETE CASCADE,
	"clicked_at" timestamptz NOT NULL DEFAULT now(),
	"ip_hash" text NOT NULL,
	"user_agent" text,
	"referer" text,
	"country" text,
	"host" text NOT NULL,
	"path" text NOT NULL,
	"query_string" text,
	"redirect_status_used" smallint NOT NULL
);
CREATE INDEX "click_events_link_idx" ON "click_events" ("link_id");
CREATE INDEX "click_events_clicked_at_idx" ON "click_events" ("clicked_at");
CREATE INDEX "click_events_link_clicked_at_idx" ON "click_events" ("link_id", "clicked_at");

-- Snarvei's own fixed-window rate limiter (redirects, invitation registration, ...).
CREATE TABLE "rate_limit" (
	"key" text PRIMARY KEY,
	"window_start" timestamptz NOT NULL,
	"count" integer NOT NULL DEFAULT 0
);

-- +goose Down

DROP TABLE "rate_limit";
DROP TABLE "click_events";
DROP TABLE "link_target_history";
DROP TABLE "links";
DROP TABLE "email_change_requests";
DROP TABLE "invitation_teams";
DROP TABLE "team_members";
DROP TABLE "teams";
DROP TABLE "organization_invitations";
DROP TABLE "organization_member_roles";
DROP TABLE "organization_members";
DROP TABLE "two_factors";
DROP TABLE "rate_limits";
DROP TABLE "verifications";
DROP TABLE "accounts";
DROP TABLE "sessions";
DROP TABLE "organizations";
DROP TABLE "users";
