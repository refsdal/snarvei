import DeleteOutlineIcon from "@mui/icons-material/Delete";
import {
  Alert,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  IconButton,
  InputLabel,
  List,
  ListItem,
  ListItemText,
  MenuItem,
  Select,
  Stack,
  Typography,
} from "@mui/material";
import { useEffect, useState } from "react";
import { authClient } from "../lib/auth-client";
import { readErrorMessage } from "../types";
import type { Member, Team, TeamMember } from "../types";

const fetchTeamMembers = async (teamId: string): Promise<{ members: TeamMember[]; error: string | null }> => {
  const response = await fetch(`/api/teams/${teamId}/members`, { credentials: "include" });
  if (!response.ok) {
    return { members: [], error: await readErrorMessage(response, "Unable to load team members.") };
  }
  return { members: (await response.json()) as TeamMember[], error: null };
};

/**
 * Manage which organization members belong to a team. Team membership is the
 * only permission boundary for org `member`s, so this is how they get access.
 */
export function TeamMembersDialog({
  team,
  organizationId,
  members,
  onClose,
  onChanged,
}: {
  team: Team | null;
  organizationId: string;
  members: Member[];
  onClose: () => void;
  onChanged?: () => void;
}) {
  const [teamMembers, setTeamMembers] = useState<TeamMember[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selectedUserId, setSelectedUserId] = useState("");
  const [busy, setBusy] = useState(false);
  const teamId = team?.id ?? null;

  useEffect(() => {
    if (!teamId) {
      return;
    }
    let cancelled = false;
    void fetchTeamMembers(teamId).then((result) => {
      if (cancelled) {
        return;
      }
      setTeamMembers(result.members);
      setError(result.error);
    });
    return () => {
      cancelled = true;
    };
  }, [teamId]);

  const reload = async (forTeamId: string) => {
    const result = await fetchTeamMembers(forTeamId);
    setTeamMembers(result.members);
    setError(result.error);
    onChanged?.();
  };

  const memberIds = new Set((teamMembers ?? []).map((member) => member.userId));
  const candidates = members.filter((member) => !memberIds.has(member.user.id));

  const add = async () => {
    if (!teamId || !selectedUserId) {
      return;
    }
    setBusy(true);
    const result = await authClient.organization.addTeamMember({ teamId, userId: selectedUserId, organizationId });
    setBusy(false);
    if (result.error) {
      setError(result.error.message ?? "Unable to add the member to the team.");
      return;
    }
    setSelectedUserId("");
    await reload(teamId);
  };

  const remove = async (userId: string) => {
    if (!teamId) {
      return;
    }
    setBusy(true);
    const result = await authClient.organization.removeTeamMember({ teamId, userId, organizationId });
    setBusy(false);
    if (result.error) {
      setError(result.error.message ?? "Unable to remove the member from the team.");
      return;
    }
    await reload(teamId);
  };

  return (
    <Dialog open={Boolean(team)} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Team members · {team?.name}</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <Typography variant="body2" color="text.secondary">
            Organization members only see links of the teams they belong to. Owners and admins always see every team.
          </Typography>
          {error ? <Alert severity="error">{error}</Alert> : null}
          {teamMembers === null ? (
            <CircularProgress size={24} />
          ) : teamMembers.length ? (
            <List dense disablePadding data-testid="team-members-list">
              {teamMembers.map((teamMember) => {
                const member = members.find((candidate) => candidate.user.id === teamMember.userId);
                return (
                  <ListItem
                    key={teamMember.id}
                    secondaryAction={
                      <IconButton
                        edge="end"
                        aria-label={`Remove ${member?.user.email ?? teamMember.userId} from team`}
                        data-testid={`remove-team-member-${teamMember.userId}`}
                        disabled={busy}
                        onClick={() => void remove(teamMember.userId)}
                      >
                        <DeleteOutlineIcon />
                      </IconButton>
                    }
                  >
                    <ListItemText
                      primary={member?.user.name ?? teamMember.userId}
                      secondary={member?.user.email ?? null}
                    />
                  </ListItem>
                );
              })}
            </List>
          ) : (
            <Alert severity="info">No members in this team yet.</Alert>
          )}
          <Stack direction="row" spacing={1} sx={{ alignItems: "center" }}>
            <FormControl fullWidth size="small">
              <InputLabel id="add-team-member-label">Add organization member</InputLabel>
              <Select
                labelId="add-team-member-label"
                label="Add organization member"
                value={selectedUserId}
                onChange={(event) => setSelectedUserId(event.target.value)}
                inputProps={{ "data-testid": "add-team-member-select" }}
              >
                {candidates.length ? (
                  candidates.map((member) => (
                    <MenuItem
                      key={member.user.id}
                      value={member.user.id}
                      data-testid={`add-team-member-option-${member.user.email}`}
                    >
                      {member.user.name} · {member.user.email}
                    </MenuItem>
                  ))
                ) : (
                  <MenuItem value="" disabled>
                    Everyone is already in this team
                  </MenuItem>
                )}
              </Select>
            </FormControl>
            <Button
              variant="contained"
              disabled={busy || !selectedUserId}
              onClick={() => void add()}
              data-testid="add-team-member-button"
            >
              Add
            </Button>
          </Stack>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Close</Button>
      </DialogActions>
    </Dialog>
  );
}
