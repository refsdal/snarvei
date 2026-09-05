import AddIcon from "@mui/icons-material/Add";
import { Box, Button, FormControl, InputLabel, MenuItem, Paper, Select, Stack, Typography } from "@mui/material";
import { DataGrid, type GridColDef, type GridRenderCellParams, type GridRowParams, Toolbar } from "@mui/x-data-grid";
import { useMemo, useState } from "react";
import { CopyButton } from "../../components/copy-button";
import { CreateLinkDialog } from "../../components/dialogs";
import { useMessage } from "../../components/message-context";
import { ApiError, errorMessage } from "../../lib/api";
import { useCreateLink, useLinks, useTeams } from "../../lib/data";
import { orgParams } from "../../lib/routes";
import { linksRoute, orgRoute } from "../../router";

export function LinksPage() {
  const navigate = linksRoute.useNavigate();
  const { organization } = orgRoute.useRouteContext();
  const { teamId, page = 1 } = linksRoute.useSearch();
  const { setMessage } = useMessage();
  const teams = useTeams(organization.id);
  const links = useLinks(organization.id, { teamId, page, pageSize: 100 });
  const createLink = useCreateLink(organization.id);
  const [createLinkOpen, setCreateLinkOpen] = useState(false);

  const appOrigin = window.location.origin;
  const activeTeamId = teamId ?? teams.data?.[0]?.id ?? null;

  const columns = useMemo<GridColDef[]>(
    () => [
      {
        field: "teamName",
        headerName: "Team",
        width: 180,
        valueGetter: (value) => value || "Unknown team",
      },
      {
        field: "fullLink",
        headerName: "Full link",
        flex: 1.2,
        minWidth: 240,
        sortable: false,
        valueGetter: (_value, row) => `${appOrigin}/l/${row.slug}`,
        renderCell: (params: GridRenderCellParams) => <CopyButton value={params.value as string} />,
      },
      {
        field: "targetUrl",
        headerName: "Destination",
        flex: 1.2,
        minWidth: 260,
      },
      {
        field: "title",
        headerName: "Title",
        flex: 1,
        minWidth: 180,
        valueGetter: (value, row) => value || row.slug,
      },
      {
        field: "redirectStatus",
        headerName: "Status code",
        width: 140,
      },
    ],
    [appOrigin],
  );

  const handleRowClick = (params: GridRowParams) => {
    void navigate({ to: "/app/$org/links/$linkId", params: { ...orgParams(organization), linkId: String(params.id) } });
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
            Links
          </Typography>
          <Typography color="text.secondary">{`Showing links you can access in ${organization.name}.`}</Typography>
        </Box>
        <Stack direction="row" spacing={1} sx={{ alignItems: "center" }}>
          {(teams.data?.length ?? 0) >= 2 ? (
            <FormControl size="small" sx={{ minWidth: 180 }}>
              <InputLabel id="links-team-filter-label">Team</InputLabel>
              <Select
                labelId="links-team-filter-label"
                label="Team"
                value={teamId ?? ""}
                inputProps={{ "data-testid": "links-team-filter" }}
                onChange={(event) => {
                  const value = event.target.value;
                  void navigate({
                    to: ".",
                    search: (prev) => ({ ...prev, teamId: value || undefined, page: undefined }),
                  });
                }}
              >
                <MenuItem value="">All teams</MenuItem>
                {teams.data?.map((team) => (
                  <MenuItem key={team.id} value={team.id}>
                    {team.name}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          ) : null}
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            disabled={!teams.data?.length}
            onClick={() => setCreateLinkOpen(true)}
          >
            Create link
          </Button>
        </Stack>
      </Stack>

      <Paper sx={{ border: "1px solid rgba(255,255,255,0.08)", p: 1.5 }}>
        <Box sx={{ height: 640 }}>
          <DataGrid
            rows={links.data?.items ?? []}
            columns={columns}
            loading={links.isPending}
            disableRowSelectionOnClick
            showToolbar
            slots={{ toolbar: Toolbar }}
            onRowClick={handleRowClick}
            paginationMode="server"
            rowCount={links.data?.total ?? 0}
            paginationModel={{ page: page - 1, pageSize: 100 }}
            onPaginationModelChange={(model) =>
              navigate({ to: ".", search: (prev) => ({ ...prev, page: model.page + 1 }) })
            }
            sx={{
              border: 0,
              "& .MuiDataGrid-cell": { cursor: "pointer" },
              "& .MuiDataGrid-columnHeaders": { backgroundColor: "rgba(255,255,255,0.02)" },
            }}
          />
        </Box>
      </Paper>

      <CreateLinkDialog
        open={createLinkOpen}
        teams={teams.data ?? []}
        activeTeamId={activeTeamId}
        appOrigin={appOrigin}
        submitting={createLink.isPending}
        onClose={() => setCreateLinkOpen(false)}
        onSubmit={async (values) => {
          try {
            const link = await createLink.mutateAsync({
              teamId: values.teamId,
              targetUrl: values.targetUrl,
              redirectStatus: values.redirectStatus,
              title: values.title,
              description: values.description,
              slug: values.slug || undefined,
            });
            setMessage({ severity: "success", text: "Short link created." });
            void navigate({ to: "/app/$org/links/$linkId", params: { ...orgParams(organization), linkId: link.id } });
            return true;
          } catch (err) {
            if (err instanceof ApiError && err.code === "SLUG_TAKEN") {
              setMessage({ severity: "error", text: err.message });
              return false;
            }
            setMessage({ severity: "error", text: errorMessage(err, "Unable to create link.") });
            return false;
          }
        }}
      />
    </Stack>
  );
}
