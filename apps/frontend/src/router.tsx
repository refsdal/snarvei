import type { QueryClient } from "@tanstack/react-query";
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  lazyRouteComponent,
  Outlet,
  redirect,
} from "@tanstack/react-router";
import { AppShell } from "./components/app-shell";
import { PageFallback } from "./components/page-fallback";
import { NotFound, RouteError } from "./components/route-error";
import { client, unwrap } from "./lib/api";
import { meQueryOptions, organizationsQueryOptions } from "./lib/data";
import { afterAuthPath } from "./lib/routes";
import { LandingPage } from "./routes/landing/page";

export type RouterContext = { queryClient: QueryClient };

export const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: () => <Outlet />,
  errorComponent: RouteError,
  notFoundComponent: NotFound,
});

type LandingSearch = { next?: string; forgot?: string; reset?: string };
const str = (v: unknown) => (typeof v === "string" ? v : undefined);

// "/" — signed-in visitors go straight to the app; the landing page keeps its
// ?next / ?forgot=1 / ?reset=done search params.
export const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  validateSearch: (s: Record<string, unknown>): LandingSearch => ({
    next: str(s.next),
    forgot: str(s.forgot),
    reset: str(s.reset),
  }),
  beforeLoad: async ({ context, search }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions());
    // `href`, not `to`: `afterAuthPath` may carry a query string (e.g. from an
    // emailed settings link), and `to` does not split on "?".
    if (me) throw redirect({ href: afterAuthPath(search.next), replace: true });
  },
  component: LandingPage,
});

export const resetPasswordRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/reset-password",
  validateSearch: (s: Record<string, unknown>): { token?: string; error?: string } => ({
    token: str(s.token),
    error: str(s.error),
  }),
  component: lazyRouteComponent(() => import("./routes/reset-password/page"), "ResetPasswordPage"),
});

// Public on purpose (spec §6): an invitee without an account registers here.
export const invitationRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/app/invitations/$invitationId",
  component: lazyRouteComponent(() => import("./routes/invitation/page"), "InvitationPage"),
});

// Everything else under /app needs a session. The destination survives the
// round trip through the landing page as ?next=.
export const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/app",
  beforeLoad: async ({ context, location }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions());
    if (!me) {
      throw redirect({
        to: "/",
        search: location.pathname.startsWith("/app/") ? { next: location.href } : {},
        replace: true,
      });
    }
    return { me };
  },
  component: AppShell,
  pendingComponent: () => <PageFallback fullScreen />,
});

export const appIndexRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/",
  component: lazyRouteComponent(() => import("./routes/organization-selection/page"), "OrganizationSelectionPage"),
});

export const settingsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/settings",
  validateSearch: (s: Record<string, unknown>): { emailToken?: string } => ({ emailToken: str(s.emailToken) }),
  component: lazyRouteComponent(() => import("./routes/settings/page"), "SettingsPage"),
});

// /app/$org: resolve the slug (or id) and make it the active organization
// server-side, so every /api call that reads the session's active org agrees
// with the URL. Unknown slug → picker.
export const orgRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/$org",
  beforeLoad: async ({ context, params }) => {
    const organizations = await context.queryClient.ensureQueryData(organizationsQueryOptions());
    const organization = organizations.find((o) => o.slug === params.org || o.id === params.org);
    if (!organization) throw redirect({ to: "/app", replace: true });
    if (context.me.session.activeOrganizationId !== organization.id) {
      await unwrap<void>(
        client.POST("/api/organizations/{orgId}/switch", { params: { path: { orgId: organization.id } } }),
      );
      await context.queryClient.invalidateQueries({ queryKey: meQueryOptions().queryKey });
    }
    return { organization };
  },
  component: () => <Outlet />,
  pendingComponent: PageFallback,
});

// Children of orgRoute are added by Tasks 4 and 5:
//   /$org/dashboard, /$org/links, /$org/links/$linkId, /$org/organization

const routeTree = rootRoute.addChildren([
  indexRoute,
  resetPasswordRoute,
  invitationRoute,
  appRoute.addChildren([appIndexRoute, settingsRoute, orgRoute.addChildren([])]),
]);

export const router = createRouter({
  routeTree,
  context: { queryClient: undefined as unknown as QueryClient }, // provided in main.tsx
  defaultPendingComponent: PageFallback,
  defaultPreload: "intent",
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
