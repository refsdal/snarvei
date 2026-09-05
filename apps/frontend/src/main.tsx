import { CssBaseline, ThemeProvider } from "@mui/material";
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { MessageProvider } from "./components/message-context";
import { queryClient } from "./lib/query";
import { router } from "./router";
import { theme } from "./theme";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <MessageProvider>
          <RouterProvider router={router} context={{ queryClient }} />
        </MessageProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
);
