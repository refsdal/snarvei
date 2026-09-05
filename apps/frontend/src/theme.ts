import { createTheme } from "@mui/material";

export const theme = createTheme({
  palette: {
    mode: "dark",
    primary: {
      main: "#8b5cf6",
    },
    secondary: {
      main: "#22d3ee",
    },
    background: {
      default: "#070b16",
      paper: "#0f172a",
    },
  },
  shape: {
    borderRadius: 16,
  },
  typography: {
    fontFamily: "Inter, system-ui, sans-serif",
  },
});
