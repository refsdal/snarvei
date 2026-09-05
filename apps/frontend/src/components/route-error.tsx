import { Alert, Box, Button, Stack, Typography } from "@mui/material";

function ErrorLayout({ title, detail, showReload }: { title: string; detail: string | null; showReload: boolean }) {
  return (
    <Box sx={{ minHeight: "100vh", display: "grid", placeItems: "center", p: 3 }}>
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
export function RouteError({ error }: { error: unknown }) {
  const detail = error instanceof Error ? error.message : null;
  return <ErrorLayout title="Something went wrong" detail={detail} showReload />;
}

export function NotFound() {
  return <ErrorLayout title="Page not found" detail={null} showReload={false} />;
}
