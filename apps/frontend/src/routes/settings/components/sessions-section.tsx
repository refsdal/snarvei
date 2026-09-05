import ShieldOutlinedIcon from "@mui/icons-material/ShieldOutlined";
import { Alert, Button, Chip, CircularProgress, List, ListItem, ListItemText, Stack, Typography } from "@mui/material";
import { errorMessage } from "../../../lib/api";
import { useRevokeOtherSessions, useRevokeSession, useSessions } from "../../../lib/data";
import type { SessionSummary } from "../../../lib/data";
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

const describeSession = (session: SessionSummary) => session.userAgent || "Browser session";

const describeActivity = (session: SessionSummary) =>
  `Created ${formatDateValue(session.createdAt)} · Last active ${
    session.lastAccess ? formatDateValue(session.lastAccess) : "never"
  } · Expires ${formatDateValue(session.expiresAt)}`;

export function SessionsSection(props: SharedSectionProps) {
  const sessions = useSessions();
  const revokeSession = useRevokeSession();
  const revokeOtherSessions = useRevokeOtherSessions();
  const sessionList = sessions.data ?? [];

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
                await revokeOtherSessions.mutateAsync();
                props.setMessage({ severity: "success", text: "Other sessions revoked." });
              })
            }
          >
            Revoke other sessions
          </Button>
        </Stack>
        {sessions.isPending ? <CircularProgress size={24} /> : null}
        {sessions.isError ? (
          <Alert severity="error">{errorMessage(sessions.error, "Unable to load active sessions.")}</Alert>
        ) : (
          <List sx={{ display: "grid", gap: 1, p: 0 }}>
            {sessionList.map((session) => (
              <ListItem
                key={session.id}
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
                    disabled={session.current || props.busyAction === `revoke-${session.id}`}
                    onClick={() =>
                      void props.runAction(`revoke-${session.id}`, async () => {
                        await revokeSession.mutateAsync(session.id);
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
                      <Typography sx={{ fontWeight: 700 }}>{describeSession(session)}</Typography>
                      {session.current ? <Chip label="Current session" size="small" color="secondary" /> : null}
                    </Stack>
                  }
                  secondary={describeActivity(session)}
                />
              </ListItem>
            ))}
            {!sessions.isPending && !sessionList.length ? (
              <Alert severity="info">No active sessions found.</Alert>
            ) : null}
          </List>
        )}
      </Stack>
    </SectionCard>
  );
}
