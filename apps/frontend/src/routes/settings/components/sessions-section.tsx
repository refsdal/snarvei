import ShieldOutlinedIcon from "@mui/icons-material/ShieldOutlined";
import { Alert, Button, Chip, CircularProgress, List, ListItem, ListItemText, Stack, Typography } from "@mui/material";
import { authClient } from "../../../lib/legacy-auth-client";
import type { AuthSession } from "../../../types";
import { SectionCard } from "./section-card";
import type { SharedSectionProps } from "./types";

const formatDateValue = (value: string | number | null | undefined) => {
  if (!value) {
    return "Unknown";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "Unknown";
  }

  return date.toLocaleString();
};

const describeSession = (session: AuthSession) => {
  const parts = [session.userAgent, session.ipAddress].filter(Boolean);
  return parts.length ? parts.join(" · ") : "Browser session";
};

export function SessionsSection(
  props: SharedSectionProps & {
    sessions: AuthSession[];
    sessionsLoading: boolean;
    currentSessionId: string;
    loadSessions: () => Promise<void>;
  },
) {
  return (
    <SectionCard
      title="Active sessions"
      description="Review current logins and revoke sessions that should no longer have access."
      icon={<ShieldOutlinedIcon />}
    >
      <Stack spacing={2}>
        <Stack
          direction={{ xs: "column", sm: "row" }}
          spacing={1.5}
          sx={{ justifyContent: "space-between", alignItems: { sm: "center" } }}
        >
          <Typography color="text.secondary">
            Only active sessions are shown. Revoked sessions disappear from this list.
          </Typography>
          <Button
            variant="outlined"
            disabled={props.busyAction === "revoke-other-sessions"}
            onClick={() =>
              void props.runAction("revoke-other-sessions", async () => {
                const result = await authClient.revokeOtherSessions();
                if (result.error) {
                  props.setMessage({
                    severity: "error",
                    text: result.error.message ?? "Unable to revoke other sessions.",
                  });
                  return;
                }
                await props.loadSessions();
                props.setMessage({ severity: "success", text: "Other sessions revoked." });
              })
            }
          >
            Revoke other sessions
          </Button>
        </Stack>
        {props.sessionsLoading ? <CircularProgress size={24} /> : null}
        <List sx={{ display: "grid", gap: 1, p: 0 }}>
          {props.sessions.map((authSession) => {
            const isCurrent = authSession.id === props.currentSessionId;
            return (
              <ListItem
                key={authSession.id}
                sx={{
                  px: 2,
                  py: 1.5,
                  border: "1px solid rgba(255,255,255,0.08)",
                  borderRadius: 3,
                  display: "flex",
                  gap: 2,
                  alignItems: "center",
                }}
                secondaryAction={
                  <Button
                    color="inherit"
                    disabled={isCurrent || props.busyAction === `revoke-${authSession.id}`}
                    onClick={() =>
                      void props.runAction(`revoke-${authSession.id}`, async () => {
                        const result = await authClient.revokeSession({ token: authSession.token });
                        if (result.error) {
                          props.setMessage({
                            severity: "error",
                            text: result.error.message ?? "Unable to revoke session.",
                          });
                          return;
                        }
                        await props.loadSessions();
                        props.setMessage({ severity: "success", text: "Session revoked." });
                      })
                    }
                  >
                    Revoke
                  </Button>
                }
              >
                <ListItemText
                  primary={
                    <Stack direction="row" spacing={1} sx={{ alignItems: "center", flexWrap: "wrap" }}>
                      <Typography sx={{ fontWeight: 700 }}>{describeSession(authSession)}</Typography>
                      {isCurrent ? <Chip label="Current session" size="small" color="secondary" /> : null}
                    </Stack>
                  }
                  secondary={`Created ${formatDateValue(authSession.createdAt)} · Expires ${formatDateValue(authSession.expiresAt)}`}
                />
              </ListItem>
            );
          })}
          {!props.sessionsLoading && !props.sessions.length ? (
            <Alert severity="info">No active sessions found.</Alert>
          ) : null}
        </List>
      </Stack>
    </SectionCard>
  );
}
