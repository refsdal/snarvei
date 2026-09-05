import { Alert, Box, Button, CircularProgress, Paper, Stack, Typography } from "@mui/material";
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useWorkspace } from "../../hooks/use-workspace-context";
import { authClient, type InvitationDetails } from "../../lib/auth-client";
import { buildOrganizationPath } from "../../lib/routes";
import { roleLabel } from "../../types";

type LoadState =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "ready"; invitation: InvitationDetails };

const fetchInvitation = async (invitationId: string): Promise<LoadState> => {
  const result = await authClient.organization.getInvitation({ query: { id: invitationId } });
  if (result.error || !result.data) {
    return {
      status: "error",
      message: result.error?.message ?? "This invitation could not be found. It may have expired or been cancelled.",
    };
  }
  return { status: "ready", invitation: result.data };
};

export function InvitationPage() {
  const { invitationId = "" } = useParams();
  const navigate = useNavigate();
  const { refreshOrganizations, session, setMessage, switchOrganization } = useWorkspace();
  const [state, setState] = useState<LoadState>({ status: "loading" });
  const [busy, setBusy] = useState<"accept" | "decline" | null>(null);

  useEffect(() => {
    let cancelled = false;
    void fetchInvitation(invitationId).then((next) => {
      if (!cancelled) {
        setState(next);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [invitationId]);

  const accept = async (invitation: InvitationDetails) => {
    setBusy("accept");
    const result = await authClient.organization.acceptInvitation({ invitationId: invitation.id });
    if (result.error) {
      setMessage({ severity: "error", text: result.error.message ?? "Unable to accept the invitation." });
      setBusy(null);
      return;
    }
    await refreshOrganizations({ silent: true });
    await switchOrganization(invitation.organizationId);
    setMessage({ severity: "success", text: `You joined ${invitation.organizationName}.` });
    void navigate(buildOrganizationPath({ id: invitation.organizationId, slug: invitation.organizationSlug }));
  };

  const decline = async (invitation: InvitationDetails) => {
    setBusy("decline");
    const result = await authClient.organization.rejectInvitation({ invitationId: invitation.id });
    if (result.error) {
      setMessage({ severity: "error", text: result.error.message ?? "Unable to decline the invitation." });
      setBusy(null);
      return;
    }
    setMessage({ severity: "info", text: "Invitation declined." });
    void navigate("/app/select-organization");
  };

  const emailMismatch =
    state.status === "ready" && session?.user.email?.toLowerCase() !== state.invitation.email.toLowerCase();

  return (
    <Box sx={{ minHeight: "100vh", display: "grid", placeItems: "center", p: 3 }}>
      <Paper
        sx={{ width: "100%", maxWidth: 560, p: 4, border: "1px solid rgba(255,255,255,0.08)" }}
        data-testid="invitation-card"
      >
        <Stack spacing={3}>
          <Typography variant="h4" sx={{ fontWeight: 800 }}>
            Organization invitation
          </Typography>
          {state.status === "loading" ? <CircularProgress /> : null}
          {state.status === "error" ? <Alert severity="error">{state.message}</Alert> : null}
          {state.status === "ready" ? (
            <>
              <Typography>
                You have been invited to join{" "}
                <strong data-testid="invitation-organization">{state.invitation.organizationName}</strong> as{" "}
                <strong>{roleLabel(state.invitation.role)}</strong>
                {state.invitation.inviterEmail ? ` by ${state.invitation.inviterEmail}` : ""}.
              </Typography>
              {state.invitation.status !== "pending" ? (
                <Alert severity="warning">
                  This invitation is {state.invitation.status} and can no longer be accepted.
                </Alert>
              ) : null}
              {emailMismatch ? (
                <Alert severity="warning">
                  This invitation was sent to {state.invitation.email}, but you are signed in as {session?.user.email}.
                  Sign in with the invited address to accept it.
                </Alert>
              ) : null}
              <Stack direction="row" spacing={2}>
                <Button
                  variant="contained"
                  data-testid="invitation-accept-button"
                  disabled={busy !== null || emailMismatch || state.invitation.status !== "pending"}
                  onClick={() => void accept(state.invitation)}
                >
                  Accept invitation
                </Button>
                <Button
                  variant="outlined"
                  data-testid="invitation-decline-button"
                  disabled={busy !== null || state.invitation.status !== "pending"}
                  onClick={() => void decline(state.invitation)}
                >
                  Decline
                </Button>
              </Stack>
            </>
          ) : null}
          <Button variant="text" onClick={() => navigate("/app/select-organization")}>
            Go to my organizations
          </Button>
        </Stack>
      </Paper>
    </Box>
  );
}
