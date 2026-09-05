import { Alert, Box, Button, CircularProgress, Paper, Stack, Typography } from "@mui/material";
import { useState } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { CreateOrganizationDialog } from "../../components/dialogs";
import { useWorkspace } from "../../hooks/use-workspace-context";
import { buildOrganizationPath } from "../../lib/routes";
import type { OrganizationSummary } from "../../types";

export function OrganizationSelectionPage() {
  const navigate = useNavigate();
  const {
    activeOrganizationId,
    createOrganization,
    loadingOrganizations,
    organizations,
    session,
    submitting,
    switchOrganization,
  } = useWorkspace();
  const [createOrganizationOpen, setCreateOrganizationOpen] = useState(false);

  if (!session) {
    return <Navigate to="/" replace />;
  }

  return (
    <Box
      sx={{
        minHeight: "100vh",
        background: "radial-gradient(circle at top, rgba(139,92,246,0.24), transparent 35%), #070b16",
        display: "grid",
        placeItems: "center",
        p: 3,
      }}
    >
      <Paper sx={{ width: "100%", maxWidth: 960, p: 4, border: "1px solid rgba(255,255,255,0.08)" }}>
        <Stack spacing={3}>
          <Box>
            <Typography variant="h3" sx={{ fontWeight: 800 }}>
              Choose your organization
            </Typography>
            <Typography color="text.secondary" sx={{ mt: 1 }}>
              Pick the workspace you want to manage. If you do not have one yet, create an organization now or wait for
              an invitation from an admin.
            </Typography>
          </Box>
          {loadingOrganizations ? <CircularProgress /> : null}
          {organizations.length ? (
            <Stack spacing={2}>
              {organizations.map((organization: OrganizationSummary) => (
                <Paper key={organization.id} sx={{ p: 3, border: "1px solid rgba(255,255,255,0.08)" }}>
                  <Stack
                    direction={{ xs: "column", sm: "row" }}
                    spacing={2}
                    sx={{ justifyContent: "space-between", alignItems: { sm: "center" } }}
                  >
                    <Box>
                      <Typography variant="h6" sx={{ fontWeight: 700 }}>
                        {organization.name}
                      </Typography>
                      <Typography color="text.secondary">/{organization.slug ?? "organization"}</Typography>
                    </Box>
                    <Button
                      variant={activeOrganizationId === organization.id ? "contained" : "outlined"}
                      onClick={() =>
                        void switchOrganization(organization.id).then(() =>
                          navigate(buildOrganizationPath(organization)),
                        )
                      }
                    >
                      Open workspace
                    </Button>
                  </Stack>
                </Paper>
              ))}
            </Stack>
          ) : (
            <Alert severity="info">
              No organizations are available yet. Create one now to get started, or wait for an invitation if an
              administrator plans to add you.
            </Alert>
          )}
          <Stack direction="row" spacing={2}>
            <Button variant="contained" onClick={() => setCreateOrganizationOpen(true)}>
              Create organization
            </Button>
          </Stack>
        </Stack>
      </Paper>
      <CreateOrganizationDialog
        open={createOrganizationOpen}
        submitting={submitting === "create-organization"}
        onClose={() => setCreateOrganizationOpen(false)}
        onSubmit={async (values) => {
          const createdOrganizationId = await createOrganization(values);
          if (createdOrganizationId) {
            void navigate(buildOrganizationPath({ id: createdOrganizationId, slug: values.slug }));
            return true;
          }
          return false;
        }}
      />
    </Box>
  );
}
