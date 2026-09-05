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
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { errorMessage } from "../lib/api";
import { teamMembersQueryOptions, useAddTeamMember, useRemoveTeamMember } from "../lib/data";
import type { Member, Team } from "../lib/data";

/**
 * Manage which organization members belong to a team. Team membership is the
 * only permission boundary for org `member`s, so this is how they get access.
 */
export function TeamMembersDialog({
  team,
  members,
  onClose,
}: {
  team: Team | null;
  members: Member[];
  onClose: () => void;
}) {
  const teamId = team?.id ?? "";
  const teamMembers = useQuery({ ...teamMembersQueryOptions(teamId), enabled: Boolean(team) });
  const addTeamMember = useAddTeamMember(teamId);
  const removeTeamMember = useRemoveTeamMember(teamId);
  const [selectedUserId, setSelectedUserId] = useState("");
  const [error, setError] = useState<string | null>(null);

  const memberIds = new Set((teamMembers.data ?? []).map((member) => member.userId));
  const candidates = members.filter((member) => !memberIds.has(member.userId));

  const add = async () => {
    if (!selectedUserId) {
      return;
    }
    setError(null);
    try {
      await addTeamMember.mutateAsync(selectedUserId);
      setSelectedUserId("");
    } catch (err) {
      setError(errorMessage(err, "Unable to add the member to the team."));
    }
  };

  const remove = async (userId: string) => {
    setError(null);
    try {
      await removeTeamMember.mutateAsync(userId);
    } catch (err) {
      setError(errorMessage(err, "Unable to remove the member from the team."));
    }
  };

  const busy = addTeamMember.isPending || removeTeamMember.isPending;

  return (
    <Dialog open={Boolean(team)} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Team members · {team?.name}</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <Typography variant="body2" color="text.secondary">
            Organization members only see links of the teams they belong to. Owners and admins always see every team.
          </Typography>
          {error ? <Alert severity="error">{error}</Alert> : null}
          {teamMembers.isPending ? (
            <CircularProgress size={24} />
          ) : teamMembers.data?.length ? (
            <List dense disablePadding data-testid="team-members-list">
              {teamMembers.data.map((teamMember) => (
                <ListItem
                  key={teamMember.userId}
                  secondaryAction={
                    <IconButton
                      edge="end"
                      aria-label={`Remove ${teamMember.email} from team`}
                      data-testid={`remove-team-member-${teamMember.userId}`}
                      disabled={busy}
                      onClick={() => void remove(teamMember.userId)}
                    >
                      <DeleteOutlineIcon />
                    </IconButton>
                  }
                >
                  <ListItemText primary={teamMember.name} secondary={teamMember.email} />
                </ListItem>
              ))}
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
                      key={member.userId}
                      value={member.userId}
                      data-testid={`add-team-member-option-${member.email}`}
                    >
                      {member.name} · {member.email}
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
