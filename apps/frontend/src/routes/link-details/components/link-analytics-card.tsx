import {
  Alert,
  Card,
  CardContent,
  CircularProgress,
  FormControl,
  MenuItem,
  Paper,
  Select,
  Stack,
  Typography,
} from "@mui/material";
import { useState } from "react";
import { useLinkAnalytics } from "../../../lib/data";

const DAY_OPTIONS = [7, 30, 90] as const;

export function LinkAnalyticsCard(props: { linkId: string }) {
  const [days, setDays] = useState<number>(30);
  const analytics = useLinkAnalytics(props.linkId, days);
  const data = analytics.data;

  return (
    <Card sx={{ flex: 1, border: "1px solid rgba(255,255,255,0.08)" }}>
      <CardContent>
        <Stack direction="row" sx={{ justifyContent: "space-between", alignItems: "center", mb: 2 }}>
          <Typography variant="h6" sx={{ fontWeight: 700 }}>
            Analytics
          </Typography>
          <FormControl size="small">
            <Select value={days} onChange={(event) => setDays(Number(event.target.value))}>
              {DAY_OPTIONS.map((option) => (
                <MenuItem key={option} value={option}>
                  Last {option} days
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Stack>
        {analytics.isPending ? <CircularProgress size={20} /> : null}
        {data ? (
          <Stack spacing={2}>
            <Stack direction="row" spacing={2}>
              <Paper sx={{ flex: 1, p: 2, border: "1px solid rgba(255,255,255,0.06)" }}>
                <Typography color="text.secondary">Total clicks</Typography>
                <Typography data-testid="analytics-total-clicks" variant="h4" sx={{ fontWeight: 800 }}>
                  {data.totalClicks}
                </Typography>
              </Paper>
              <Paper sx={{ flex: 1, p: 2, border: "1px solid rgba(255,255,255,0.06)" }}>
                <Typography color="text.secondary">Unique visitors</Typography>
                <Typography data-testid="analytics-unique-visitors" variant="h4" sx={{ fontWeight: 800 }}>
                  {data.uniqueVisitorApproximation}
                </Typography>
              </Paper>
            </Stack>
            <Stack spacing={1}>
              <Typography variant="subtitle2">Top countries</Typography>
              {data.topCountries.length ? (
                data.topCountries.map((entry) => (
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
            {data.topReferrers.length ? (
              <Stack spacing={1}>
                <Typography variant="subtitle2">Top referrers</Typography>
                {data.topReferrers.map((entry) => (
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
            {data.clicksByDay.length ? (
              <Stack spacing={1}>
                <Typography variant="subtitle2">Clicks per day</Typography>
                <Paper sx={{ p: 2, border: "1px solid rgba(255,255,255,0.06)" }}>
                  <Stack spacing={0.5}>
                    {data.clicksByDay.map((entry) => (
                      <Stack key={entry.day} direction="row" sx={{ justifyContent: "space-between" }}>
                        <Typography variant="body2">{entry.day}</Typography>
                        <Typography variant="body2" color="text.secondary">
                          {entry.clicks}
                        </Typography>
                      </Stack>
                    ))}
                  </Stack>
                </Paper>
                {data.range.from ? (
                  <Typography variant="caption" color="text.secondary">
                    Window: {new Date(data.range.from).toLocaleDateString()} –{" "}
                    {new Date(data.range.to).toLocaleDateString()}
                  </Typography>
                ) : null}
              </Stack>
            ) : null}
          </Stack>
        ) : null}
      </CardContent>
    </Card>
  );
}
