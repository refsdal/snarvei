import AddIcon from "@mui/icons-material/Add";
import GroupIcon from "@mui/icons-material/Group";
import PersonAddAlt1Icon from "@mui/icons-material/PersonAddAlt1";
import { Alert, Box, Button, Card, CardContent, Chip, CircularProgress, Paper, Stack, Typography } from "@mui/material";
import { DataGrid, type GridColDef, Toolbar } from "@mui/x-data-grid";
import { useMemo, useState } from "react";
import { CreateTeamDialog, InviteMemberDialog } from "../../components/dialogs";
import { TeamMembersDialog } from "../../components/team-members-dialog";
import { useWorkspace } from "../../hooks/use-workspace-context";
import { roleLabel } from "../../types";
import type { Invitation, Team } from "../../types";

export function OrganizationPage() {
  const {
    activeOrganization,
    activeOrganizationId,
    cancelInvitation,
    createTeam,
    invitations,
    loadingInvitations,
    loadingMembers,
    members,
    refreshOrganizationData,
    submitting,
    teams,
    inviteMember,
  } = useWorkspace();
  const [inviteOpen, setInviteOpen] = useState(false);
  const [createTeamOpen, setCreateTeamOpen] = useState(false);
  const [managedTeam, setManagedTeam] = useState<Team | null>(null);
  const teamName = (teamId?: string | null) => teams.find((team) => team.id === teamId)?.name ?? null;

  const columns = useMemo<GridColDef[]>(
    () => [
      {
        field: "name",
        headerName: "Name",
        flex: 1,
        minWidth: 200,
        valueGetter: (_value, row) => row.user.name,
      },
      {
        field: "email",
        headerName: "Email",
        flex: 1.1,
        minWidth: 240,
        valueGetter: (_value, row) => row.user.email,
      },
      {
        field: "role",
        headerName: "Role",
        width: 180,
        valueGetter: (value) => roleLabel(value),
      },
    ],
    [],
  );

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
            Manage members, invitations, and teams for {activeOrganization?.name ?? "the active organization"}.
          </Typography>
        </Box>
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
      </Stack>

      <Paper sx={{ border: "1px solid rgba(255,255,255,0.08)", p: 1.5 }}>
        <Box sx={{ height: 540 }}>
          <DataGrid
            rows={members}
            columns={columns}
            loading={loadingMembers}
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
              {teams.length ? (
                teams.map((team: Team) => (
                  <Chip
                    key={team.id}
                    label={team.name}
                    icon={<GroupIcon />}
                    onClick={() => {
                      // Refresh members first: someone may have accepted an invitation since the page loaded.
                      if (activeOrganizationId) {
                        void refreshOrganizationData(activeOrganizationId, { silent: true });
                      }
                      setManagedTeam(team);
                    }}
                    data-testid={`manage-team-${team.name}`}
                    clickable
                  />
                ))
              ) : (
                <Alert severity="info">No teams in this organization yet.</Alert>
              )}
            </Stack>
            {teams.length ? (
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
            {loadingInvitations ? <CircularProgress size={20} /> : null}
            <Stack spacing={1}>
              {invitations.length ? (
                invitations.map((invitation: Invitation) => (
                  <Paper
                    key={invitation.id}
                    sx={{ p: 2, border: "1px solid rgba(255,255,255,0.06)" }}
                    data-testid={`invitation-${invitation.email}`}
                  >
                    <Stack direction="row" spacing={2} sx={{ justifyContent: "space-between", alignItems: "center" }}>
                      <Box>
                        <Typography sx={{ fontWeight: 700 }}>{invitation.email}</Typography>
                        <Typography variant="body2" color="text.secondary">
                          {roleLabel(invitation.role)} · {invitation.status}
                          {teamName(invitation.teamId) ? ` · team ${teamName(invitation.teamId)}` : ""}
                        </Typography>
                      </Box>
                      {invitation.status === "pending" ? (
                        <Button
                          size="small"
                          color="inherit"
                          disabled={submitting === `cancel-${invitation.id}`}
                          onClick={() => void cancelInvitation(invitation.id)}
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
        submitting={submitting === "invite-member"}
        teams={teams}
        onClose={() => setInviteOpen(false)}
        onSubmit={async (values) => inviteMember(values)}
      />
      <TeamMembersDialog
        team={managedTeam}
        organizationId={activeOrganizationId ?? ""}
        members={members}
        onClose={() => setManagedTeam(null)}
        onChanged={() => {
          if (activeOrganizationId) {
            void refreshOrganizationData(activeOrganizationId, { silent: true });
          }
        }}
      />
      <CreateTeamDialog
        open={createTeamOpen}
        submitting={submitting === "create-team"}
        onClose={() => setCreateTeamOpen(false)}
        onSubmit={async (values) => Boolean(await createTeam(values))}
      />
    </Stack>
  );
}
