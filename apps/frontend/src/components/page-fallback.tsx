import { Box, CircularProgress } from "@mui/material";

export const PageFallback = ({ fullScreen = false }: { fullScreen?: boolean }) => (
  <Box sx={{ minHeight: fullScreen ? "100vh" : 240, display: "grid", placeItems: "center" }}>
    <CircularProgress />
  </Box>
);
