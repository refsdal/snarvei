import { Box, Card, CardContent, Stack, Typography } from "@mui/material";

export function SectionCard(props: {
  title: string;
  description: string;
  children: React.ReactNode;
  icon?: React.ReactNode;
}) {
  return (
    <Card sx={{ border: "1px solid rgba(255,255,255,0.08)", background: "rgba(15,23,42,0.72)" }}>
      <CardContent sx={{ p: 3.5 }}>
        <Stack spacing={2.5}>
          <Stack direction="row" spacing={1.5} sx={{ alignItems: "center" }}>
            {props.icon ? (
              <Box sx={{ color: "secondary.main", display: "grid", placeItems: "center" }}>{props.icon}</Box>
            ) : null}
            <Box>
              <Typography variant="h5" sx={{ fontWeight: 800 }}>
                {props.title}
              </Typography>
              <Typography color="text.secondary">{props.description}</Typography>
            </Box>
          </Stack>
          {props.children}
        </Stack>
      </CardContent>
    </Card>
  );
}
