import { Alert, Card, CardContent, CircularProgress, Paper, Stack, Typography } from "@mui/material";
import { useLinkHistory } from "../../../lib/data";

export function LinkHistoryCard(props: { linkId: string }) {
  const history = useLinkHistory(props.linkId);
  const items = history.data?.items ?? [];

  return (
    <Card sx={{ flex: 1, border: "1px solid rgba(255,255,255,0.08)" }}>
      <CardContent>
        <Stack direction="row" sx={{ justifyContent: "space-between", alignItems: "center", mb: 2 }}>
          <Typography variant="h6" sx={{ fontWeight: 700 }}>
            History
          </Typography>
          {history.data ? (
            <Typography variant="body2" color="text.secondary">
              {history.data.total} change{history.data.total === 1 ? "" : "s"}
            </Typography>
          ) : null}
        </Stack>
        {history.isPending ? <CircularProgress size={20} /> : null}
        <Stack spacing={1}>
          {items.length ? (
            items.map((item) => (
              <Paper key={item.id} sx={{ p: 2, border: "1px solid rgba(255,255,255,0.06)" }}>
                <Typography variant="body2" color="text.secondary">
                  {new Date(item.changedAt).toLocaleString()}
                </Typography>
                <Typography sx={{ fontWeight: 700 }}>{item.newTargetUrl}</Typography>
                <Typography variant="body2" color="text.secondary">
                  Previous target: {item.oldTargetUrl ?? "Initial value"}
                </Typography>
              </Paper>
            ))
          ) : !history.isPending ? (
            <Alert severity="info">No target history for the selected link yet.</Alert>
          ) : null}
        </Stack>
      </CardContent>
    </Card>
  );
}
