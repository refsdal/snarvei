import { Alert, Card, CardContent, CircularProgress, Paper, Stack, Typography } from "@mui/material";
import type { HistoryItem } from "../../../types";

export function LinkHistoryCard(props: { history: HistoryItem[]; loading: boolean }) {
  return (
    <Card sx={{ flex: 1, border: "1px solid rgba(255,255,255,0.08)" }}>
      <CardContent>
        <Typography variant="h6" sx={{ fontWeight: 700, mb: 2 }}>
          History
        </Typography>
        {props.loading ? <CircularProgress size={20} /> : null}
        <Stack spacing={1}>
          {props.history.length ? (
            props.history.map((item) => (
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
          ) : (
            <Alert severity="info">No target history for the selected link yet.</Alert>
          )}
        </Stack>
      </CardContent>
    </Card>
  );
}
