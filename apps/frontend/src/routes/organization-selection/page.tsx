import { Alert, Box, Button, CircularProgress, Paper, Stack, Typography } from "@mui/material";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { CreateOrganizationDialog } from "../../components/dialogs";
import { useMessage } from "../../components/message-context";
import { errorMessage } from "../../lib/api";
import type { Organization } from "../../lib/data";
import { useCreateOrganization, useMe, useOrganizations } from "../../lib/data";
import { orgParams } from "../../lib/routes";

export function OrganizationSelectionPage() {
  const navigate = useNavigate();
  const { data: me } = useMe();
  const { data: organizations = [], isPending } = useOrganizations();
  const createOrganization = useCreateOrganization();
  const { setMessage } = useMessage();
  const activeOrganizationId = me?.session.activeOrganizationId ?? null;
  const [createOrganizationOpen, setCreateOrganizationOpen] = useState(false);

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
          {isPending ? <CircularProgress /> : null}
          {organizations.length ? (
            <Stack spacing={2}>
              {organizations.map((organization: Organization) => (
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
                      onClick={() => void navigate({ to: "/app/$org/dashboard", params: orgParams(organization) })}
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
        submitting={createOrganization.isPending}
        onClose={() => setCreateOrganizationOpen(false)}
        onSubmit={async (values) => {
          try {
            const created = await createOrganization.mutateAsync(values);
            void navigate({ to: "/app/$org/dashboard", params: orgParams(created) });
            return true;
          } catch (err) {
            setMessage({ severity: "error", text: errorMessage(err, "Unable to create organization.") });
            return false;
          }
        }}
      />
    </Box>
  );
}
