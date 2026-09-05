import { Alert, Box, Button, CircularProgress, Paper, Stack, TextField, Typography } from "@mui/material";
import { useQuery } from "@tanstack/react-query";
import { createLink, getRouteApi, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useMessage } from "../../components/message-context";
import { ApiError, errorMessage } from "../../lib/api";
import {
  organizationsQueryOptions,
  useAcceptInvitation,
  useMe,
  usePublicInvitation,
  useRegisterWithInvitation,
  useRejectInvitation,
} from "../../lib/data";

// `createLink` (not `component={Link}`) is what keeps `search` typed against
// the target route when the link renders as a MUI Button.
const LinkButton = createLink(Button);

const route = getRouteApi("/app/invitations/$invitationId");

export function InvitationPage() {
  const { invitationId } = route.useParams();
  const navigate = useNavigate();
  const { setMessage } = useMessage();
  const invitation = usePublicInvitation(invitationId);
  const me = useMe();
  // Warm the organizations cache for signed-in visitors: after accepting, the
  // picker on "/app" should already list the new organization.
  useQuery({ ...organizationsQueryOptions(), enabled: Boolean(me.data) });

  const acceptInvitation = useAcceptInvitation();
  const rejectInvitation = useRejectInvitation();
  const registerWithInvitation = useRegisterWithInvitation();

  const [busy, setBusy] = useState<"accept" | "decline" | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [registerError, setRegisterError] = useState<string | null>(null);
  const [emailTaken, setEmailTaken] = useState(false);
  const [invalidated, setInvalidated] = useState(false);

  const accept = async () => {
    setBusy("accept");
    setActionError(null);
    try {
      await acceptInvitation.mutateAsync(invitationId);
      setMessage({
        severity: "success",
        text: `You joined ${invitation.data?.organizationName ?? "the organization"}.`,
      });
      void navigate({ to: "/app" });
    } catch (err) {
      setActionError(errorMessage(err, "Unable to accept the invitation."));
      setBusy(null);
    }
  };

  const decline = async () => {
    setBusy("decline");
    setActionError(null);
    try {
      await rejectInvitation.mutateAsync(invitationId);
      void navigate({ to: "/app" });
    } catch (err) {
      setActionError(errorMessage(err, "Unable to decline the invitation."));
      setBusy(null);
    }
  };

  const register = async () => {
    setRegisterError(null);
    setEmailTaken(false);
    try {
      await registerWithInvitation.mutateAsync({ invitationId, name, password });
      void navigate({ to: "/app" });
    } catch (err) {
      if (err instanceof ApiError && err.code === "EMAIL_TAKEN") {
        setEmailTaken(true);
        return;
      }
      if (err instanceof ApiError && err.code === "INVITATION_INVALID") {
        setInvalidated(true);
        return;
      }
      setRegisterError(errorMessage(err, "Unable to create the account."));
    }
  };

  const loading = invitation.isPending || me.isPending;
  const notFound = invitation.isError;
  const noLongerValid = invalidated || (invitation.data ? invitation.data.status !== "pending" : false);
  const signInLink = (
    <LinkButton to="/" search={{ next: `/app/invitations/${invitationId}` }} variant="contained">
      Sign in
    </LinkButton>
  );

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

          {loading ? <CircularProgress /> : null}

          {!loading && notFound ? (
            <Alert severity="error">This invitation could not be found. It may have expired or been cancelled.</Alert>
          ) : null}

          {!loading && !notFound && invitation.data && noLongerValid ? (
            <Alert severity="warning">This invitation is no longer valid.</Alert>
          ) : null}

          {!loading && !notFound && invitation.data && !noLongerValid ? (
            me.data ? (
              <>
                <Typography>
                  You have been invited to join{" "}
                  <strong data-testid="invitation-organization">{invitation.data.organizationName}</strong> as{" "}
                  <strong>{invitation.data.role}</strong>
                  {invitation.data.inviterName ? ` by ${invitation.data.inviterName}` : ""}
                  {invitation.data.teamName ? `, joining the ${invitation.data.teamName} team` : ""}.
                </Typography>
                {actionError ? <Alert severity="error">{actionError}</Alert> : null}
                <Stack direction="row" spacing={2}>
                  <Button
                    variant="contained"
                    data-testid="invitation-accept-button"
                    disabled={busy !== null}
                    onClick={() => void accept()}
                  >
                    Accept invitation
                  </Button>
                  <Button
                    variant="outlined"
                    data-testid="invitation-decline-button"
                    disabled={busy !== null}
                    onClick={() => void decline()}
                  >
                    Decline
                  </Button>
                </Stack>
              </>
            ) : invitation.data.hasAccount ? (
              <>
                <Typography>
                  You have been invited to join{" "}
                  <strong data-testid="invitation-organization">{invitation.data.organizationName}</strong>.
                </Typography>
                <Typography color="text.secondary">Sign in with the invited address to continue.</Typography>
                {signInLink}
              </>
            ) : (
              <>
                <Typography>
                  You have been invited to join{" "}
                  <strong data-testid="invitation-organization">{invitation.data.organizationName}</strong> as{" "}
                  <strong>{invitation.data.role}</strong>. Create an account to accept.
                </Typography>
                {emailTaken ? (
                  <Alert severity="warning" action={signInLink}>
                    An account with that email exists; sign in instead.
                  </Alert>
                ) : null}
                {registerError ? <Alert severity="error">{registerError}</Alert> : null}
                <TextField
                  label="Your name"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  autoComplete="name"
                  fullWidth
                  slotProps={{ htmlInput: { "data-testid": "auth-name-input" } }}
                />
                <TextField
                  label="Password"
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  autoComplete="new-password"
                  helperText="At least 8 characters."
                  fullWidth
                  slotProps={{ htmlInput: { "data-testid": "auth-password-input", minLength: 8 } }}
                />
                <Button
                  variant="contained"
                  data-testid="create-account-button"
                  disabled={registerWithInvitation.isPending || !name.trim() || password.length < 8}
                  onClick={() => void register()}
                >
                  Create account
                </Button>
              </>
            )
          ) : null}
        </Stack>
      </Paper>
    </Box>
  );
}
