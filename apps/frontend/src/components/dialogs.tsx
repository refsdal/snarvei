import DeleteOutlineIcon from "@mui/icons-material/DeleteOutlineOutlined";
import EditOutlinedIcon from "@mui/icons-material/EditOutlined";
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormHelperText,
  InputAdornment,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
} from "@mui/material";
import { useState } from "react";
import type { Link, Team } from "../lib/data";
import type { InvitationRole } from "../lib/roles";
import type { SelectedLinkFormValues } from "../types";

export function CreateOrganizationDialog({
  open,
  submitting,
  onClose,
  onSubmit,
}: {
  open: boolean;
  submitting: boolean;
  onClose: () => void;
  onSubmit: (values: { name: string; slug: string }) => Promise<boolean>;
}) {
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Create organization</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <TextField
            label="Organization name"
            placeholder="Acme Inc"
            value={name}
            onChange={(event) => setName(event.target.value)}
            slotProps={{ htmlInput: { "data-testid": "organization-name-input" } }}
          />
          <TextField
            label="Organization slug"
            placeholder="acme"
            value={slug}
            onChange={(event) => setSlug(event.target.value)}
            slotProps={{ htmlInput: { "data-testid": "organization-slug-input" } }}
          />
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained"
          disabled={submitting || !name.trim() || !slug.trim()}
          data-testid="create-organization-button"
          onClick={() => {
            void onSubmit({ name, slug }).then((created) => {
              if (created) {
                onClose();
              }
            });
          }}
        >
          Create organization
        </Button>
      </DialogActions>
    </Dialog>
  );
}

export function CreateTeamDialog({
  open,
  submitting,
  onClose,
  onSubmit,
}: {
  open: boolean;
  submitting: boolean;
  onClose: () => void;
  onSubmit: (values: { name: string }) => Promise<boolean>;
}) {
  const [name, setName] = useState("");

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Create team</DialogTitle>
      <DialogContent>
        <TextField
          sx={{ mt: 1 }}
          label="Team name"
          placeholder="Marketing"
          value={name}
          onChange={(event) => setName(event.target.value)}
          slotProps={{ htmlInput: { "data-testid": "team-name-input" } }}
          fullWidth
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained"
          disabled={submitting || !name.trim()}
          data-testid="create-team-button"
          onClick={() => {
            void onSubmit({ name }).then((created) => {
              if (created) {
                onClose();
              }
            });
          }}
        >
          Create team
        </Button>
      </DialogActions>
    </Dialog>
  );
}

export type CreateLinkFormValues = {
  teamId: string;
  targetUrl: string;
  slug?: string;
  redirectStatus: 301 | 302 | 307;
  title?: string;
  description?: string;
};

export function CreateLinkDialog({
  open,
  teams,
  activeTeamId,
  appOrigin,
  submitting,
  onClose,
  onSubmit,
}: {
  open: boolean;
  teams: Team[];
  activeTeamId: string | null;
  appOrigin: string;
  submitting: boolean;
  onClose: () => void;
  onSubmit: (values: CreateLinkFormValues) => Promise<boolean>;
}) {
  const defaultTeamId = activeTeamId ?? teams[0]?.id ?? "";

  return (
    <CreateLinkDialogForm
      key={`${open}-${defaultTeamId}`}
      open={open}
      defaultTeamId={defaultTeamId}
      appOrigin={appOrigin}
      submitting={submitting}
      teams={teams}
      onClose={onClose}
      onSubmit={onSubmit}
    />
  );
}

function CreateLinkDialogForm({
  open,
  defaultTeamId,
  appOrigin,
  submitting,
  teams,
  onClose,
  onSubmit,
}: {
  open: boolean;
  defaultTeamId: string;
  appOrigin: string;
  submitting: boolean;
  teams: Team[];
  onClose: () => void;
  onSubmit: (values: CreateLinkFormValues) => Promise<boolean>;
}) {
  const [teamId, setTeamId] = useState(defaultTeamId);
  const [targetUrl, setTargetUrl] = useState("");
  const [slug, setSlug] = useState("");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [redirectStatus, setRedirectStatus] = useState<301 | 302 | 307>(302);

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Create link</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <FormControl>
            <InputLabel id="create-link-team-label">Team</InputLabel>
            <Select
              labelId="create-link-team-label"
              label="Team"
              value={teamId}
              onChange={(event) => setTeamId(event.target.value)}
            >
              {teams.map((team) => (
                <MenuItem key={team.id} value={team.id}>
                  {team.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <TextField
            label="Target URL"
            placeholder="https://example.com/landing-page"
            value={targetUrl}
            onChange={(event) => setTargetUrl(event.target.value)}
            slotProps={{ htmlInput: { "data-testid": "create-link-target-input" } }}
          />
          <TextField
            label="Custom slug (optional)"
            placeholder="q3-launch"
            value={slug}
            onChange={(event) => setSlug(event.target.value)}
            helperText="Leave blank to generate one. Lowercase letters, digits and single hyphens, 3–64 characters."
            slotProps={{
              htmlInput: { "data-testid": "create-link-slug-input", autoCapitalize: "none", spellCheck: false },
              input: { startAdornment: <InputAdornment position="start">{`${appOrigin}/l/`}</InputAdornment> },
            }}
          />
          <TextField
            label="Title"
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            slotProps={{ htmlInput: { "data-testid": "create-link-title-input" } }}
          />
          <TextField
            label="Description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            multiline
            minRows={2}
            slotProps={{ htmlInput: { "data-testid": "create-link-description-input" } }}
          />
          <FormControl>
            <InputLabel id="create-link-status-label">Redirect status</InputLabel>
            <Select
              labelId="create-link-status-label"
              label="Redirect status"
              value={redirectStatus}
              onChange={(event) => setRedirectStatus(event.target.value)}
            >
              <MenuItem value={302}>302 — temporary (default)</MenuItem>
              <MenuItem value={307}>307 — temporary, preserves request method</MenuItem>
              <MenuItem value={301}>301 — permanent (browsers may cache it and ignore later target changes)</MenuItem>
            </Select>
          </FormControl>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained"
          disabled={!teamId || !targetUrl.trim() || submitting}
          data-testid="create-link-button"
          onClick={() => {
            void onSubmit({
              teamId,
              targetUrl,
              slug: slug.trim() || undefined,
              redirectStatus,
              title,
              description,
            }).then((created) => {
              if (created) {
                onClose();
              }
            });
          }}
        >
          Create link
        </Button>
      </DialogActions>
    </Dialog>
  );
}

export function InviteMemberDialog({
  open,
  submitting,
  teams,
  onClose,
  onSubmit,
}: {
  open: boolean;
  submitting: boolean;
  teams: Team[];
  onClose: () => void;
  onSubmit: (values: { email: string; role: InvitationRole; teamId?: string | null }) => Promise<boolean>;
}) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<InvitationRole>("member");
  const [teamId, setTeamId] = useState<string>("");

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Invite member</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <TextField
            label="Invite email"
            placeholder="colleague@example.com"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            slotProps={{ htmlInput: { "data-testid": "invite-email-input" } }}
          />
          <FormControl>
            <InputLabel id="invite-role-label">Role</InputLabel>
            <Select
              labelId="invite-role-label"
              label="Role"
              value={role}
              onChange={(event) => setRole(event.target.value)}
            >
              <MenuItem value="member">member</MenuItem>
              <MenuItem value="admin">admin</MenuItem>
            </Select>
          </FormControl>
          <FormControl>
            <InputLabel id="invite-team-label">Team (optional)</InputLabel>
            <Select
              labelId="invite-team-label"
              label="Team (optional)"
              value={teamId}
              onChange={(event) => setTeamId(event.target.value)}
              inputProps={{ "data-testid": "invite-team-select" }}
            >
              <MenuItem value="">No team yet</MenuItem>
              {teams.map((team) => (
                <MenuItem key={team.id} value={team.id} data-testid={`invite-team-option-${team.name}`}>
                  {team.name}
                </MenuItem>
              ))}
            </Select>
            <FormHelperText>
              Members only see links of teams they belong to; owners and admins see everything.
            </FormHelperText>
          </FormControl>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained"
          disabled={submitting || !email.trim()}
          data-testid="send-invitation-button"
          onClick={() => {
            void onSubmit({ email, role, teamId: teamId || null }).then((created) => {
              if (created) {
                onClose();
              }
            });
          }}
        >
          Send invitation
        </Button>
      </DialogActions>
    </Dialog>
  );
}

export function EditLinkDialog({
  open,
  link,
  submitting,
  onClose,
  onSubmit,
  onDelete,
}: {
  open: boolean;
  link: Link;
  submitting: string | null;
  onClose: () => void;
  onSubmit: (values: SelectedLinkFormValues) => Promise<boolean>;
  onDelete: () => Promise<boolean>;
}) {
  return (
    <EditLinkDialogForm
      key={`${link.id}-${link.updatedAt}-${open}`}
      open={open}
      link={link}
      submitting={submitting}
      onClose={onClose}
      onSubmit={onSubmit}
      onDelete={onDelete}
    />
  );
}

function EditLinkDialogForm({
  open,
  link,
  submitting,
  onClose,
  onSubmit,
  onDelete,
}: {
  open: boolean;
  link: Link;
  submitting: string | null;
  onClose: () => void;
  onSubmit: (values: SelectedLinkFormValues) => Promise<boolean>;
  onDelete: () => Promise<boolean>;
}) {
  const [targetUrl, setTargetUrl] = useState(link.targetUrl);
  const [title, setTitle] = useState(link.title ?? "");
  const [description, setDescription] = useState(link.description ?? "");
  const [redirectStatus, setRedirectStatus] = useState<301 | 302 | 307>(link.redirectStatus);
  const [isActive, setIsActive] = useState(link.isActive);

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Edit link</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <TextField
            label="Target URL"
            value={targetUrl}
            onChange={(event) => setTargetUrl(event.target.value)}
            slotProps={{ htmlInput: { "data-testid": "selected-link-target-input" } }}
          />
          <TextField
            label="Title"
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            slotProps={{ htmlInput: { "data-testid": "selected-link-title-input" } }}
          />
          <TextField
            label="Description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            multiline
            minRows={2}
            slotProps={{ htmlInput: { "data-testid": "selected-link-description-input" } }}
          />
          <FormControl>
            <InputLabel id="selected-link-status-label">Redirect status</InputLabel>
            <Select
              labelId="selected-link-status-label"
              label="Redirect status"
              value={redirectStatus}
              onChange={(event) => setRedirectStatus(event.target.value)}
            >
              <MenuItem value={302}>302 — temporary (default)</MenuItem>
              <MenuItem value={307}>307 — temporary, preserves request method</MenuItem>
              <MenuItem value={301}>301 — permanent (browsers may cache it and ignore later target changes)</MenuItem>
            </Select>
          </FormControl>
          <FormControl>
            <InputLabel id="selected-link-active-label">Active state</InputLabel>
            <Select
              labelId="selected-link-active-label"
              label="Active state"
              value={isActive ? "active" : "inactive"}
              onChange={(event) => setIsActive(event.target.value === "active")}
            >
              <MenuItem value="active">Active</MenuItem>
              <MenuItem value="inactive">Inactive</MenuItem>
            </Select>
          </FormControl>
        </Stack>
      </DialogContent>
      <DialogActions sx={{ justifyContent: "space-between", px: 3, pb: 2 }}>
        <Button
          color="error"
          startIcon={<DeleteOutlineIcon />}
          disabled={submitting === "delete-link"}
          data-testid="delete-link-button"
          onClick={() => {
            void onDelete().then((deleted) => {
              if (deleted) {
                onClose();
              }
            });
          }}
        >
          Delete link
        </Button>
        <Stack direction="row" spacing={1}>
          <Button onClick={onClose}>Cancel</Button>
          <Button
            variant="contained"
            startIcon={<EditOutlinedIcon />}
            disabled={submitting === "update-link"}
            data-testid="save-link-button"
            onClick={() => {
              void onSubmit({ targetUrl, title, description, redirectStatus, isActive }).then((saved) => {
                if (saved) {
                  onClose();
                }
              });
            }}
          >
            Save changes
          </Button>
        </Stack>
      </DialogActions>
    </Dialog>
  );
}
