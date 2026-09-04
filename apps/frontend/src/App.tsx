import {
  Alert,
  Box,
  Button,
  CircularProgress,
  CssBaseline,
  Stack,
  ThemeProvider,
  Typography,
  createTheme,
} from "@mui/material";
import { Suspense, lazy } from "react";
import {
  createBrowserRouter,
  Navigate,
  Outlet,
  RouterProvider,
  isRouteErrorResponse,
  useLocation,
  useParams,
  useRouteError,
} from "react-router-dom";
import { AppShell } from "./components/app-shell";
import { WorkspaceProvider } from "./hooks/use-workspace";
import { useWorkspace } from "./hooks/use-workspace-context";
import { buildOrganizationPath, settingsPath } from "./lib/routes";
import { LandingPage } from "./routes/landing/page";

// Route-level code splitting: the landing page stays in the main bundle, every
// authenticated page (MUI data grids, QR codes, settings) loads on demand.
const DashboardPage = lazy(() => import("./routes/dashboard/page").then((m) => ({ default: m.DashboardPage })));
const LinkDetailsPage = lazy(() => import("./routes/link-details/page").then((m) => ({ default: m.LinkDetailsPage })));
const LinksPage = lazy(() => import("./routes/links/page").then((m) => ({ default: m.LinksPage })));
const OrganizationPage = lazy(() =>
  import("./routes/organization/page").then((m) => ({ default: m.OrganizationPage })),
);
const InvitationPage = lazy(() => import("./routes/invitation/page").then((m) => ({ default: m.InvitationPage })));
const OrganizationSelectionPage = lazy(() =>
  import("./routes/organization-selection/page").then((m) => ({ default: m.OrganizationSelectionPage })),
);
const SettingsPage = lazy(() => import("./routes/settings/page").then((m) => ({ default: m.SettingsPage })));
const ResetPasswordPage = lazy(() =>
  import("./routes/reset-password/page").then((m) => ({ default: m.ResetPasswordPage })),
);

const PageFallback = () => (
  <Box sx={{ minHeight: 240, display: "grid", placeItems: "center" }}>
    <CircularProgress />
  </Box>
);

const theme = createTheme({
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

/** Route error boundary: a render/loader error no longer blanks the whole app. */
function RouteError() {
  const error = useRouteError();
  const title = isRouteErrorResponse(error) ? `${error.status} ${error.statusText}` : "Something went wrong";
  const detail = isRouteErrorResponse(error) ? null : error instanceof Error ? error.message : null;
  return (
    <Box sx={{ minHeight: "100vh", display: "grid", placeItems: "center", p: 3 }}>
      <Stack spacing={2} sx={{ maxWidth: 520 }}>
        <Typography variant="h4" sx={{ fontWeight: 800 }}>
          {title}
        </Typography>
        <Alert severity="error">
          {detail ?? "The page could not be rendered. Try reloading, or go back to your workspace."}
        </Alert>
        <Stack direction="row" spacing={1}>
          <Button variant="contained" onClick={() => window.location.reload()}>
            Reload
          </Button>
          <Button variant="outlined" href="/app">
            Go to workspace
          </Button>
        </Stack>
      </Stack>
    </Box>
  );
}

function RequireSession() {
  const { session, sessionPending } = useWorkspace();
  const location = useLocation();

  if (sessionPending) {
    return (
      <Box sx={{ minHeight: "100vh", display: "grid", placeItems: "center" }}>
        <CircularProgress />
      </Box>
    );
  }

  if (!session) {
    // Keep the destination (e.g. an invitation link) through sign-in/sign-up.
    const next = `${location.pathname}${location.search}`;
    return <Navigate to={next.startsWith("/app/") ? `/?next=${encodeURIComponent(next)}` : "/"} replace />;
  }

  return (
    <Suspense fallback={<PageFallback />}>
      <Outlet />
    </Suspense>
  );
}

function OrganizationIndexRedirect() {
  const { org } = useParams();
  const { activeOrganization, organizations, organizationsPending } = useWorkspace();

  if (org) {
    const routeOrganization = organizations.find(
      (organization) => organization.slug === org || organization.id === org,
    );
    if (routeOrganization) {
      return <Navigate to={buildOrganizationPath(routeOrganization)} replace />;
    }

    if (organizationsPending) {
      return null;
    }
  }

  if (!activeOrganization) {
    return <Navigate to="/app/select-organization" replace />;
  }

  return <Navigate to={buildOrganizationPath(activeOrganization)} replace />;
}

// Created once at module scope: recreating the router on re-render would remount the whole tree.
const router = createBrowserRouter([
  {
    path: "/",
    element: <LandingPage />,
    errorElement: <RouteError />,
  },
  {
    // Public: reached from the password-reset email, the user has no session yet.
    path: "/reset-password",
    element: <ResetPasswordPage />,
    errorElement: <RouteError />,
  },
  {
    element: <RequireSession />,
    errorElement: <RouteError />,
    children: [
      {
        path: "/app/select-organization",
        element: <OrganizationSelectionPage />,
      },
      {
        path: "/app/invitations/:invitationId",
        element: <InvitationPage />,
      },
      {
        path: settingsPath,
        element: <AppShell />,
        children: [
          {
            index: true,
            element: <SettingsPage />,
          },
        ],
      },
      {
        path: "/app",
        element: <AppShell />,
        children: [
          {
            index: true,
            element: <OrganizationIndexRedirect />,
          },
          {
            path: ":org",
            children: [
              {
                index: true,
                element: <OrganizationIndexRedirect />,
              },
              {
                path: "dashboard",
                element: <DashboardPage />,
              },
              {
                path: "links",
                element: <LinksPage />,
              },
              {
                path: "links/:linkId",
                element: <LinkDetailsPage />,
              },
              {
                path: "organization",
                element: <OrganizationPage />,
              },
            ],
          },
        ],
      },
    ],
  },
]);

export function App() {
  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <WorkspaceProvider>
        <RouterProvider router={router} />
      </WorkspaceProvider>
    </ThemeProvider>
  );
}
