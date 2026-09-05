import { Alert, Box, Button, Stack, Typography } from "@mui/material";

function ErrorLayout({
  title,
  detail,
  showReload,
  fullScreen = true,
}: {
  title: string;
  detail: string | null;
  showReload: boolean;
  fullScreen?: boolean;
}) {
  return (
    <Box sx={{ minHeight: fullScreen ? "100vh" : 240, display: "grid", placeItems: "center", p: 3 }}>
      <Stack spacing={2} sx={{ maxWidth: 520 }}>
        <Typography variant="h4" sx={{ fontWeight: 800 }}>
          {title}
        </Typography>
        <Alert severity="error">
          {detail ??
            (showReload
              ? "The page could not be rendered. Try reloading, or go back to your workspace."
              : "This page does not exist.")}
        </Alert>
        <Stack direction="row" spacing={1}>
          {showReload ? (
            <Button variant="contained" onClick={() => window.location.reload()}>
              Reload
            </Button>
          ) : null}
          <Button variant="outlined" href="/app">
            Go to workspace
          </Button>
        </Stack>
      </Stack>
    </Box>
  );
}

/** Route error boundary: a render/loader error no longer blanks the whole app. */
export function RouteError({ error, fullScreen = true }: { error: unknown; fullScreen?: boolean }) {
  const detail = error instanceof Error ? error.message : null;
  return <ErrorLayout title="Something went wrong" detail={detail} showReload fullScreen={fullScreen} />;
}

export function NotFound({ fullScreen = true }: { fullScreen?: boolean } = {}) {
  return <ErrorLayout title="Page not found" detail={null} showReload={false} fullScreen={fullScreen} />;
}
