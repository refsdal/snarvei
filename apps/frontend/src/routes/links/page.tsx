import AddIcon from "@mui/icons-material/Add";
import { Box, Button, Paper, Stack, Typography } from "@mui/material";
import { DataGrid, type GridColDef, type GridRenderCellParams, type GridRowParams, Toolbar } from "@mui/x-data-grid";
import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { CopyButton } from "../../components/copy-button";
import { CreateLinkDialog } from "../../components/dialogs";
import { useWorkspace } from "../../hooks/use-workspace-context";
import { buildLinksPath } from "../../lib/routes";

export function LinksPage() {
  const navigate = useNavigate();
  const { activeOrganization, appOrigin, createLink, links, loadingLinks, submitting, teams, activeTeamId } =
    useWorkspace();
  const [createLinkOpen, setCreateLinkOpen] = useState(false);

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
    void navigate(buildLinksPath(activeOrganization, String(params.id)));
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
          <Typography color="text.secondary">
            {activeOrganization
              ? `Showing links you can access in ${activeOrganization.name}.`
              : "Choose an organization to start managing links."}
          </Typography>
        </Box>
        <Stack direction="row" spacing={1}>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            disabled={!teams.length}
            onClick={() => setCreateLinkOpen(true)}
          >
            Create link
          </Button>
        </Stack>
      </Stack>

      <Paper sx={{ border: "1px solid rgba(255,255,255,0.08)", p: 1.5 }}>
        <Box sx={{ height: 640 }}>
          <DataGrid
            rows={links}
            columns={columns}
            loading={loadingLinks}
            disableRowSelectionOnClick
            showToolbar
            slots={{ toolbar: Toolbar }}
            onRowClick={handleRowClick}
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
        teams={teams}
        activeTeamId={activeTeamId ?? teams[0]?.id ?? null}
        appOrigin={appOrigin}
        submitting={submitting === "create-link"}
        onClose={() => setCreateLinkOpen(false)}
        onSubmit={async (values) => {
          const createdLink = await createLink(values);
          if (createdLink) {
            void navigate(buildLinksPath(activeOrganization, createdLink.id));
            return true;
          }
          return false;
        }}
      />
    </Stack>
  );
}
