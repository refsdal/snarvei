import AddIcon from "@mui/icons-material/Add";
import PersonAddAlt1Icon from "@mui/icons-material/PersonAddAlt1";
import { Alert, Box, Button, Card, CardContent, CircularProgress, Paper, Stack, Typography } from "@mui/material";
import { useNavigate } from "@tanstack/react-router";
import { useInvitations, useLinks, useMembers, useTeams } from "../../lib/data";
import { buildLinksPath, buildOrganizationPath } from "../../lib/routes";
import { orgRoute } from "../../router";

const StatCard = (props: { label: string; value: number; testId: string }) => (
  <Paper sx={{ flex: 1, p: 2.5, border: "1px solid rgba(255,255,255,0.08)" }}>
    <Typography color="text.secondary">{props.label}</Typography>
    <Typography variant="h4" sx={{ fontWeight: 800 }} data-testid={props.testId}>
      {props.value}
    </Typography>
  </Paper>
);

export function DashboardPage() {
  const navigate = useNavigate();
  const { organization } = orgRoute.useRouteContext();
  const links = useLinks(organization.id, { page: 1, pageSize: 100 });
  const teams = useTeams(organization.id);
  const members = useMembers(organization.id);
  const invitations = useInvitations(organization.id);

  const recentLinks = links.data?.items.slice(0, 5) ?? [];
  const pendingInvitations = invitations.data?.filter((invitation) => invitation.status === "pending").length ?? 0;
  const loadingLinks = links.isPending;

  return (
    <Stack spacing={3}>
      <Stack
        direction={{ xs: "column", md: "row" }}
        spacing={2}
        sx={{ justifyContent: "space-between", alignItems: { md: "center" } }}
      >
        <Box>
          <Typography variant="h4" sx={{ fontWeight: 800 }}>
            Overview
          </Typography>
          <Typography color="text.secondary">{`What is happening in ${organization.name}.`}</Typography>
        </Box>
        <Stack direction="row" spacing={1}>
          <Button
            variant="outlined"
            startIcon={<PersonAddAlt1Icon />}
            onClick={() => navigate({ to: buildOrganizationPath(organization, "organization") })}
          >
            Invite member
          </Button>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            disabled={!teams.data?.length}
            onClick={() => navigate({ to: buildLinksPath(organization) })}
          >
            Create link
          </Button>
        </Stack>
      </Stack>

      <Stack direction={{ xs: "column", md: "row" }} spacing={2}>
        <StatCard label="Short links" value={links.data?.total ?? 0} testId="dashboard-links-count" />
        <StatCard label="Teams" value={teams.data?.length ?? 0} testId="dashboard-teams-count" />
        <StatCard label="Members" value={members.data?.length ?? 0} testId="dashboard-members-count" />
        <StatCard label="Pending invitations" value={pendingInvitations} testId="dashboard-invitations-count" />
      </Stack>

      <Card sx={{ border: "1px solid rgba(255,255,255,0.08)" }}>
        <CardContent>
          <Stack direction="row" sx={{ justifyContent: "space-between", alignItems: "center", mb: 2 }}>
            <Typography variant="h6" sx={{ fontWeight: 700 }}>
              Recent links
            </Typography>
            <Button size="small" onClick={() => navigate({ to: buildLinksPath(organization) })}>
              View all
            </Button>
          </Stack>
          {loadingLinks ? <CircularProgress size={20} /> : null}
          {!loadingLinks && !recentLinks.length ? (
            <Alert severity="info">
              {teams.data?.length
                ? "No links yet. Create your first short link from the Links page."
                : "Create a team first, then add links to it."}
            </Alert>
          ) : null}
          <Stack spacing={1}>
            {recentLinks.map((link) => (
              <Paper
                key={link.id}
                sx={{ p: 2, border: "1px solid rgba(255,255,255,0.06)", cursor: "pointer" }}
                onClick={() => navigate({ to: buildLinksPath(organization, link.id) })}
              >
                <Stack direction={{ xs: "column", sm: "row" }} spacing={1} sx={{ justifyContent: "space-between" }}>
                  <Box sx={{ minWidth: 0 }}>
                    <Typography sx={{ fontWeight: 700 }}>{link.title ?? link.slug}</Typography>
                    <Typography
                      variant="body2"
                      color="text.secondary"
                      sx={{ overflow: "hidden", textOverflow: "ellipsis" }}
                    >
                      {link.targetUrl}
                    </Typography>
                  </Box>
                  <Typography variant="body2" color="text.secondary">
                    {link.teamName ?? ""} · {link.isActive ? "active" : "inactive"} · {link.redirectStatus}
                  </Typography>
                </Stack>
              </Paper>
            ))}
          </Stack>
        </CardContent>
      </Card>
    </Stack>
  );
}
