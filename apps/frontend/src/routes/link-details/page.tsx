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
import { getRouteApi } from "@tanstack/react-router";
import { useState } from "react";
import { EditLinkDialog } from "../../components/dialogs";
import { useMessage } from "../../components/message-context";
import { ApiError, errorMessage } from "../../lib/api";
import { useDeleteLink, useLink, useUpdateLink } from "../../lib/data";
import { orgParams } from "../../lib/routes";
import { NotFound } from "../../components/route-error";
import { LinkAnalyticsCard, LinkHistoryCard } from "./components";

const route = getRouteApi("/app/$org/links/$linkId");
const orgRouteApi = getRouteApi("/app/$org");

export function LinkDetailsPage() {
  const navigate = route.useNavigate();
  const { linkId } = route.useParams();
  const { organization } = orgRouteApi.useRouteContext();
  const { setMessage } = useMessage();
  const link = useLink(linkId);
  const updateLink = useUpdateLink(organization.id);
  const deleteLink = useDeleteLink(organization.id);
  const [editOpen, setEditOpen] = useState(false);

  const appOrigin = window.location.origin;

  if (link.isPending) {
    return (
      <Box sx={{ minHeight: 240, display: "grid", placeItems: "center" }}>
        <CircularProgress />
      </Box>
    );
  }

  if (link.isError) {
    if (link.error instanceof ApiError && link.error.status === 404) {
      return <NotFound fullScreen={false} />;
    }
    return <Alert severity="error">{errorMessage(link.error, "Unable to load the selected link.")}</Alert>;
  }

  const data = link.data;

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
            onClick={() => navigate({ to: "/app/$org/links", params: orgParams(organization) })}
            sx={{ mb: 1, px: 0 }}
          >
            Back to links
          </Button>
          <Typography variant="h4" sx={{ fontWeight: 800 }}>
            {data.title || data.slug}
          </Typography>
          <Typography color="text.secondary">{data.targetUrl}</Typography>
        </Box>
        <Stack direction="row" spacing={1}>
          <Button startIcon={<OpenInNewIcon />} href={`/l/${data.slug}`} target="_blank" rel="noreferrer">
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
                  <Typography sx={{ fontFamily: "monospace" }}>{`${appOrigin}/l/${data.slug}`}</Typography>
                  <IconButton
                    size="small"
                    onClick={() => void navigator.clipboard.writeText(`${appOrigin}/l/${data.slug}`)}
                  >
                    <ContentCopyIcon fontSize="inherit" />
                  </IconButton>
                </Stack>
              </Box>
              <Box sx={{ flex: 1 }}>
                <Typography color="text.secondary">Destination</Typography>
                <Typography>{data.targetUrl}</Typography>
              </Box>
            </Stack>
            <Stack direction={{ xs: "column", md: "row" }} spacing={2}>
              <Box sx={{ flex: 1 }}>
                <Typography color="text.secondary">Status code</Typography>
                <Typography>{data.redirectStatus}</Typography>
              </Box>
              <Box sx={{ flex: 1 }}>
                <Typography color="text.secondary">State</Typography>
                <Chip
                  size="small"
                  label={data.isActive ? "Active" : "Inactive"}
                  color={data.isActive ? "success" : "default"}
                  sx={{ mt: 0.5 }}
                />
              </Box>
            </Stack>
          </Stack>
        </Stack>
      </Paper>

      <Stack direction={{ xs: "column", xl: "row" }} spacing={3}>
        <LinkHistoryCard linkId={data.id} />
        <LinkAnalyticsCard linkId={data.id} />
      </Stack>

      <EditLinkDialog
        open={editOpen}
        link={data}
        submitting={updateLink.isPending ? "update-link" : deleteLink.isPending ? "delete-link" : null}
        onClose={() => setEditOpen(false)}
        onSubmit={async (values) => {
          try {
            await updateLink.mutateAsync({
              linkId: data.id,
              targetUrl: values.targetUrl,
              title: values.title || "",
              description: values.description || "",
              redirectStatus: values.redirectStatus,
              isActive: values.isActive,
            });
            setMessage({ severity: "success", text: "Link updated." });
            return true;
          } catch (err) {
            setMessage({ severity: "error", text: errorMessage(err, "Unable to update the selected link.") });
            return false;
          }
        }}
        onDelete={async () => {
          try {
            await deleteLink.mutateAsync(data.id);
            setMessage({ severity: "success", text: "Link deleted." });
            void navigate({ to: "/app/$org/links", params: orgParams(organization) });
            return true;
          } catch (err) {
            setMessage({ severity: "error", text: errorMessage(err, "Unable to delete the selected link.") });
            return false;
          }
        }}
      />
    </Stack>
  );
}
