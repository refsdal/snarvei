import ArrowBackRoundedIcon from "@mui/icons-material/ArrowBackRounded";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  IconButton,
  Paper,
  Stack,
  Typography,
} from "@mui/material";
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { EditLinkDialog } from "../../components/dialogs";
import { useWorkspace } from "../../hooks/use-workspace-context";
import { buildLinksPath } from "../../lib/routes";
import { LinkAnalyticsCard, LinkHistoryCard } from "./components";

export function LinkDetailsPage() {
  const navigate = useNavigate();
  const params = useParams();
  const {
    activeOrganization,
    analytics,
    appOrigin,
    deleteLink,
    getLinkById,
    history,
    loadingDetails,
    loadingLinks,
    setSelectedLinkId,
    submitting,
    updateLink,
  } = useWorkspace();
  const [editOpen, setEditOpen] = useState(false);

  const link = params.linkId ? getLinkById(params.linkId) : null;

  useEffect(() => {
    setSelectedLinkId(params.linkId ?? null);
  }, [params.linkId, setSelectedLinkId]);

  if (!link && loadingLinks) {
    return (
      <Box sx={{ minHeight: 240, display: "grid", placeItems: "center" }}>
        <CircularProgress />
      </Box>
    );
  }

  if (!link) {
    return (
      <Alert severity="info">
        The selected link is not available in the current organization. Go back to the links grid and choose another
        row.
      </Alert>
    );
  }

  return (
    <Stack spacing={3}>
      <Stack
        direction={{ xs: "column", md: "row" }}
        spacing={2}
        sx={{ justifyContent: "space-between", alignItems: { md: "center" } }}
      >
        <Box>
          <Button
            startIcon={<ArrowBackRoundedIcon />}
            onClick={() => navigate(buildLinksPath(activeOrganization))}
            sx={{ mb: 1, px: 0 }}
          >
            Back to links
          </Button>
          <Typography variant="h4" sx={{ fontWeight: 800 }}>
            {link.title || link.slug}
          </Typography>
          <Typography color="text.secondary">{link.targetUrl}</Typography>
        </Box>
        <Stack direction="row" spacing={1}>
          <Button startIcon={<OpenInNewIcon />} href={`/l/${link.slug}`} target="_blank" rel="noreferrer">
            Open
          </Button>
          <Button variant="contained" onClick={() => setEditOpen(true)}>
            Edit link
          </Button>
        </Stack>
      </Stack>

      <Paper sx={{ p: 3, border: "1px solid rgba(255,255,255,0.08)" }}>
        <Stack spacing={2}>
          <Typography variant="h6" sx={{ fontWeight: 700 }}>
            Link details
          </Typography>
          <Divider />
          <Stack spacing={1.5}>
            <Stack direction={{ xs: "column", md: "row" }} spacing={2}>
              <Box sx={{ flex: 1 }}>
                <Typography color="text.secondary">Full link</Typography>
                <Stack direction="row" spacing={1} sx={{ alignItems: "center" }}>
                  <Typography sx={{ fontFamily: "monospace" }}>{`${appOrigin}/l/${link.slug}`}</Typography>
                  <IconButton
                    size="small"
                    onClick={() => void navigator.clipboard.writeText(`${appOrigin}/l/${link.slug}`)}
                  >
                    <ContentCopyIcon fontSize="inherit" />
                  </IconButton>
                </Stack>
              </Box>
              <Box sx={{ flex: 1 }}>
                <Typography color="text.secondary">Destination</Typography>
                <Typography>{link.targetUrl}</Typography>
              </Box>
            </Stack>
            <Stack direction={{ xs: "column", md: "row" }} spacing={2}>
              <Box sx={{ flex: 1 }}>
                <Typography color="text.secondary">Status code</Typography>
                <Typography>{link.redirectStatus}</Typography>
              </Box>
              <Box sx={{ flex: 1 }}>
                <Typography color="text.secondary">State</Typography>
                <Chip
                  size="small"
                  label={link.isActive ? "Active" : "Inactive"}
                  color={link.isActive ? "success" : "default"}
                  sx={{ mt: 0.5 }}
                />
              </Box>
            </Stack>
          </Stack>
        </Stack>
      </Paper>

      <Stack direction={{ xs: "column", xl: "row" }} spacing={3}>
        <LinkHistoryCard history={history} loading={loadingDetails} />
        <LinkAnalyticsCard analytics={analytics} loading={loadingDetails} />
      </Stack>

      <EditLinkDialog
        open={editOpen}
        link={link}
        submitting={submitting}
        onClose={() => setEditOpen(false)}
        onSubmit={async (values) => Boolean(await updateLink(link.id, values))}
        onDelete={async () => {
          const deleted = await deleteLink(link.id);
          if (deleted) {
            void navigate(buildLinksPath(activeOrganization));
          }
          return deleted;
        }}
      />
    </Stack>
  );
}
