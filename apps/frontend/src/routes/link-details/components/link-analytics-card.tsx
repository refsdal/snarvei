import { Alert, Card, CardContent, CircularProgress, Paper, Stack, Typography } from "@mui/material";
import type { AnalyticsSummary } from "../../../types";

export function LinkAnalyticsCard(props: { analytics: AnalyticsSummary; loading: boolean }) {
  return (
    <Card sx={{ flex: 1, border: "1px solid rgba(255,255,255,0.08)" }}>
      <CardContent>
        <Typography variant="h6" sx={{ fontWeight: 700, mb: 2 }}>
          Analytics
        </Typography>
        {props.loading ? <CircularProgress size={20} /> : null}
        <Stack spacing={2}>
          <Stack direction="row" spacing={2}>
            <Paper sx={{ flex: 1, p: 2, border: "1px solid rgba(255,255,255,0.06)" }}>
              <Typography color="text.secondary">Total clicks</Typography>
              <Typography data-testid="analytics-total-clicks" variant="h4" sx={{ fontWeight: 800 }}>
                {props.analytics.totalClicks}
              </Typography>
            </Paper>
            <Paper sx={{ flex: 1, p: 2, border: "1px solid rgba(255,255,255,0.06)" }}>
              <Typography color="text.secondary">Unique visitors</Typography>
              <Typography data-testid="analytics-unique-visitors" variant="h4" sx={{ fontWeight: 800 }}>
                {props.analytics.uniqueVisitorApproximation}
              </Typography>
            </Paper>
          </Stack>
          <Stack spacing={1}>
            <Typography variant="subtitle2">Top countries</Typography>
            {props.analytics.topCountries.length ? (
              props.analytics.topCountries.map((entry) => (
                <Paper
                  key={`${entry.country}-${entry.clicks}`}
                  sx={{ p: 2, border: "1px solid rgba(255,255,255,0.06)" }}
                >
                  <Stack direction="row" sx={{ justifyContent: "space-between" }}>
                    <Typography>{entry.country ?? "Unknown"}</Typography>
                    <Typography color="text.secondary">{entry.clicks}</Typography>
                  </Stack>
                </Paper>
              ))
            ) : (
              <Alert severity="info">No click analytics recorded yet.</Alert>
            )}
          </Stack>
          {props.analytics.topReferrers.length ? (
            <Stack spacing={1}>
              <Typography variant="subtitle2">Top referrers</Typography>
              {props.analytics.topReferrers.map((entry) => (
                <Paper
                  key={`${entry.referer}-${entry.clicks}`}
                  sx={{ p: 2, border: "1px solid rgba(255,255,255,0.06)" }}
                >
                  <Stack direction="row" sx={{ justifyContent: "space-between", gap: 2 }}>
                    <Typography sx={{ overflow: "hidden", textOverflow: "ellipsis" }}>
                      {entry.referer ?? "Direct / unknown"}
                    </Typography>
                    <Typography color="text.secondary">{entry.clicks}</Typography>
                  </Stack>
                </Paper>
              ))}
            </Stack>
          ) : null}
          {props.analytics.clicksByDay.length ? (
            <Stack spacing={1}>
              <Typography variant="subtitle2">Clicks per day</Typography>
              <Paper sx={{ p: 2, border: "1px solid rgba(255,255,255,0.06)" }}>
                <Stack spacing={0.5}>
                  {props.analytics.clicksByDay.map((entry) => (
                    <Stack key={entry.day} direction="row" sx={{ justifyContent: "space-between" }}>
                      <Typography variant="body2">{entry.day}</Typography>
                      <Typography variant="body2" color="text.secondary">
                        {entry.clicks}
                      </Typography>
                    </Stack>
                  ))}
                </Stack>
              </Paper>
              {props.analytics.range.from ? (
                <Typography variant="caption" color="text.secondary">
                  Window: {new Date(props.analytics.range.from).toLocaleDateString()} –{" "}
                  {new Date(props.analytics.range.to).toLocaleDateString()}
                </Typography>
              ) : null}
            </Stack>
          ) : null}
        </Stack>
      </CardContent>
    </Card>
  );
}
