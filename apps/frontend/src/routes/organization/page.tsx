import AddIcon from "@mui/icons-material/Add";
import GroupIcon from "@mui/icons-material/Group";
import PersonAddAlt1Icon from "@mui/icons-material/PersonAddAlt1";
import { Alert, Box, Button, Card, CardContent, Chip, CircularProgress, Paper, Stack, Typography } from "@mui/material";
import { DataGrid, type GridColDef, Toolbar } from "@mui/x-data-grid";
import { useMemo, useState } from "react";
import { CreateTeamDialog, InviteMemberDialog } from "../../components/dialogs";
import { useMessage } from "../../components/message-context";
import { TeamMembersDialog } from "../../components/team-members-dialog";
import { errorMessage } from "../../lib/api";
import {
  useCancelInvitation,
  useCreateInvitation,
  useCreateTeam,
  useInvitations,
  useMembers,
  useTeams,
} from "../../lib/data";
import type { Invitation, Member, Team } from "../../lib/data";
import { orgRoute } from "../../router";

export function OrganizationPage() {
  const { organization } = orgRoute.useRouteContext();
  const { setMessage } = useMessage();
  const members = useMembers(organization.id);
  const teams = useTeams(organization.id);
  const invitations = useInvitations(organization.id);
  const createTeam = useCreateTeam(organization.id);
  const createInvitation = useCreateInvitation(organization.id);
  const cancelInvitation = useCancelInvitation(organization.id);

  const [inviteOpen, setInviteOpen] = useState(false);
  const [createTeamOpen, setCreateTeamOpen] = useState(false);
  const [managedTeam, setManagedTeam] = useState<Team | null>(null);
  const [cancellingId, setCancellingId] = useState<string | null>(null);

  const canManage = organization.role === "owner" || organization.role === "admin";

  const columns = useMemo<GridColDef<Member>[]>(
    () => [
      { field: "name", headerName: "Name", flex: 1, minWidth: 200 },
      { field: "email", headerName: "Email", flex: 1.1, minWidth: 240 },
      { field: "role", headerName: "Role", width: 180 },
    ],
    [],
  );

  const cancel = async (invitation: Invitation) => {
    setCancellingId(invitation.id);
    try {
      await cancelInvitation.mutateAsync(invitation.id);
    } catch (err) {
      setMessage({ severity: "error", text: errorMessage(err, "Unable to cancel the invitation.") });
    } finally {
      setCancellingId(null);
    }
  };

  return (
    <Stack spacing={3}>
      <Stack
        direction={{ xs: "column", md: "row" }}
        spacing={2}
        sx={{ justifyContent: "space-between", alignItems: { md: "center" } }}
      >
        <Box>
          <Typography variant="h4" sx={{ fontWeight: 800 }}>
            Organization
          </Typography>
          <Typography color="text.secondary">
            Manage members, invitations, and teams for {organization.name}.
          </Typography>
        </Box>
        {canManage ? (
          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              startIcon={<AddIcon />}
              onClick={() => setCreateTeamOpen(true)}
              data-testid="open-create-team-button"
            >
              Create team
            </Button>
            <Button
              variant="contained"
              startIcon={<PersonAddAlt1Icon />}
              onClick={() => setInviteOpen(true)}
              data-testid="open-invite-member-button"
            >
              Invite member
            </Button>
          </Stack>
        ) : null}
      </Stack>

      <Paper sx={{ border: "1px solid rgba(255,255,255,0.08)", p: 1.5 }}>
        <Box sx={{ height: 540 }}>
          <DataGrid
            rows={members.data ?? []}
            columns={columns}
            loading={members.isPending}
            showToolbar
            slots={{ toolbar: Toolbar }}
            sx={{ border: 0 }}
          />
        </Box>
      </Paper>

      <Stack direction={{ xs: "column", xl: "row" }} spacing={3}>
        <Card sx={{ flex: 1, border: "1px solid rgba(255,255,255,0.08)" }}>
          <CardContent>
            <Typography variant="h6" sx={{ fontWeight: 700, mb: 2 }}>
              Teams
            </Typography>
            <Stack direction="row" spacing={1} sx={{ flexWrap: "wrap", gap: 1 }}>
              {teams.data?.length ? (
                teams.data.map((team) => (
                  <Chip
                    key={team.id}
                    label={team.name}
                    icon={<GroupIcon />}
                    onClick={() => setManagedTeam(team)}
                    data-testid={`manage-team-${team.name}`}
                    clickable
                  />
                ))
              ) : (
                <Alert severity="info">No teams in this organization yet.</Alert>
              )}
            </Stack>
            {teams.data?.length ? (
              <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 1 }}>
                Click a team to manage its members.
              </Typography>
            ) : null}
          </CardContent>
        </Card>

        <Card sx={{ flex: 1, border: "1px solid rgba(255,255,255,0.08)" }}>
          <CardContent>
            <Typography variant="h6" sx={{ fontWeight: 700, mb: 2 }}>
              Pending invitations
            </Typography>
            {invitations.isPending ? <CircularProgress size={20} /> : null}
            <Stack spacing={1}>
              {invitations.data?.length ? (
                invitations.data.map((invitation) => (
                  <Paper
                    key={invitation.id}
                    sx={{ p: 2, border: "1px solid rgba(255,255,255,0.06)" }}
                    data-testid={`invitation-${invitation.email}`}
                  >
                    <Stack direction="row" spacing={2} sx={{ justifyContent: "space-between", alignItems: "center" }}>
                      <Box>
                        <Typography sx={{ fontWeight: 700 }}>{invitation.email}</Typography>
                        <Typography variant="body2" color="text.secondary">
                          {invitation.role} · {invitation.status}
                          {invitation.teamName ? ` · team ${invitation.teamName}` : ""}
                        </Typography>
                      </Box>
                      {canManage && invitation.status === "pending" ? (
                        <Button
                          size="small"
                          color="inherit"
                          disabled={cancellingId === invitation.id}
                          onClick={() => void cancel(invitation)}
                          data-testid={`cancel-invitation-${invitation.email}`}
                        >
                          Cancel
                        </Button>
                      ) : null}
                    </Stack>
                  </Paper>
                ))
              ) : (
                <Alert severity="info">No pending invitations.</Alert>
              )}
            </Stack>
          </CardContent>
        </Card>
      </Stack>

      <InviteMemberDialog
        open={inviteOpen}
        submitting={createInvitation.isPending}
        teams={teams.data ?? []}
        onClose={() => setInviteOpen(false)}
        onSubmit={async ({ email, role, teamId }) => {
          try {
            await createInvitation.mutateAsync({ email, role, teamId: teamId || undefined });
            setMessage({ severity: "success", text: `Invitation sent to ${email}.` });
            return true;
          } catch (err) {
            setMessage({ severity: "error", text: errorMessage(err, "Unable to send the invitation.") });
            return false;
          }
        }}
      />
      <TeamMembersDialog team={managedTeam} members={members.data ?? []} onClose={() => setManagedTeam(null)} />
      <CreateTeamDialog
        open={createTeamOpen}
        submitting={createTeam.isPending}
        onClose={() => setCreateTeamOpen(false)}
        onSubmit={async ({ name }) => {
          try {
            await createTeam.mutateAsync({ name });
            return true;
          } catch (err) {
            setMessage({ severity: "error", text: errorMessage(err, "Unable to create the team.") });
            return false;
          }
        }}
      />
    </Stack>
  );
}
