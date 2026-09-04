import BusinessOutlinedIcon from "@mui/icons-material/BusinessOutlined";
import DashboardOutlinedIcon from "@mui/icons-material/DashboardOutlined";
import LinkOutlinedIcon from "@mui/icons-material/LinkOutlined";
import LogoutIcon from "@mui/icons-material/Logout";
import SettingsOutlinedIcon from "@mui/icons-material/SettingsOutlined";
import {
  Alert,
  Avatar,
  Box,
  Divider,
  Drawer,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  MenuItem,
  Select,
  Stack,
  Typography,
} from "@mui/material";
import { useEffect } from "react";
import { Link, Outlet, useLocation, useNavigate, useParams } from "react-router-dom";
import { useWorkspace } from "../hooks/use-workspace-context";
import { buildOrganizationPath, getOrganizationPathSegment, settingsPath } from "../lib/routes";
import type { OrganizationSummary } from "../types";

const drawerWidth = 280;

const matchesNavItem = (pathname: string, to?: string) => {
  if (!to) {
    return false;
  }

  return pathname === to || pathname.startsWith(`${to}/`);
};

export function AppShell() {
  const navigate = useNavigate();
  const location = useLocation();
  const { org } = useParams();
  const { activeOrganizationId, message, organizations, session, setMessage, signOut, switchOrganization } =
    useWorkspace();
  const activeOrganization = organizations.find((organization) => organization.id === activeOrganizationId) ?? null;
  const currentOrgSegment = org ?? getOrganizationPathSegment(activeOrganization);
  const navItems = [
    {
      to: currentOrgSegment ? `/app/${currentOrgSegment}/dashboard` : undefined,
      label: "Dashboard",
      icon: <DashboardOutlinedIcon />,
    },
    {
      to: currentOrgSegment ? `/app/${currentOrgSegment}/links` : undefined,
      label: "Links",
      icon: <LinkOutlinedIcon />,
    },
    {
      to: currentOrgSegment ? `/app/${currentOrgSegment}/organization` : undefined,
      label: "Organization",
      icon: <BusinessOutlinedIcon />,
    },
    { to: settingsPath, label: "Settings", icon: <SettingsOutlinedIcon /> },
  ];

  useEffect(() => {
    if (!org || !organizations.length) {
      return;
    }

    const routeOrganization = organizations.find(
      (organization) => organization.slug === org || organization.id === org,
    );
    if (!routeOrganization || routeOrganization.id === activeOrganizationId) {
      return;
    }

    void switchOrganization(routeOrganization.id);
  }, [activeOrganizationId, org, organizations, switchOrganization]);

  return (
    <Box sx={{ minHeight: "100vh", background: "#070b16", display: "flex" }}>
      <Drawer
        variant="permanent"
        sx={{
          width: drawerWidth,
          flexShrink: 0,
          "& .MuiDrawer-paper": {
            width: drawerWidth,
            boxSizing: "border-box",
            borderRight: "1px solid rgba(255,255,255,0.08)",
            background: "linear-gradient(180deg, rgba(7,11,22,0.98), rgba(15,23,42,0.96))",
          },
        }}
      >
        <Stack sx={{ height: "100%", p: 2.5 }}>
          <Stack direction="row" spacing={2} sx={{ alignItems: "center", mb: 4 }}>
            <Box
              sx={{
                width: 44,
                height: 44,
                borderRadius: 3,
                background: "linear-gradient(135deg, #8b5cf6, #22d3ee)",
              }}
            />
            <Box>
              <Typography variant="h6" sx={{ fontWeight: 800 }}>
                Snarvei
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Admin workspace
              </Typography>
            </Box>
          </Stack>

          <List sx={{ gap: 1, display: "grid" }}>
            {navItems.map((item) => {
              const selected = matchesNavItem(location.pathname, item.to);
              return (
                <ListItemButton
                  key={item.label}
                  component={Link}
                  to={item.to ?? "#"}
                  disabled={!item.to}
                  selected={selected}
                  sx={{
                    borderRadius: 3,
                    px: 1.5,
                    py: 1,
                    "&.Mui-selected": {
                      backgroundColor: "rgba(139,92,246,0.18)",
                    },
                  }}
                >
                  <ListItemIcon sx={{ color: "inherit", minWidth: 40 }}>{item.icon}</ListItemIcon>
                  <ListItemText primary={item.label} />
                </ListItemButton>
              );
            })}
          </List>

          <Box sx={{ flexGrow: 1 }} />

          <Divider sx={{ borderColor: "rgba(255,255,255,0.08)", mb: 2 }} />

          <Stack spacing={1.5}>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ letterSpacing: 1.2, textTransform: "uppercase" }}
            >
              Viewing organization
            </Typography>
            <Select
              size="small"
              value={activeOrganizationId ?? ""}
              displayEmpty
              onChange={(event) => {
                const organizationId = event.target.value;
                if (typeof organizationId === "string" && organizationId) {
                  void switchOrganization(organizationId).then(() => {
                    const nextOrganization = organizations.find((organization) => organization.id === organizationId);
                    void navigate(buildOrganizationPath(nextOrganization));
                  });
                }
              }}
            >
              {organizations.map((organization: OrganizationSummary) => (
                <MenuItem key={organization.id} value={organization.id}>
                  {organization.name}
                </MenuItem>
              ))}
            </Select>
            <Stack direction="row" spacing={1.5} sx={{ alignItems: "center", pt: 1 }}>
              <Avatar src={session?.user.image ?? undefined} sx={{ bgcolor: "primary.main" }}>
                {session?.user.name?.[0]?.toUpperCase() ?? "S"}
              </Avatar>
              <Box sx={{ minWidth: 0, flexGrow: 1 }}>
                <Typography sx={{ fontWeight: 700 }} noWrap>
                  {session?.user.name ?? "Signed in user"}
                </Typography>
                <Typography variant="body2" color="text.secondary" noWrap>
                  {session?.user.email ?? ""}
                </Typography>
              </Box>
              <ListItemButton
                onClick={() => void signOut().then(() => navigate("/"))}
                sx={{ borderRadius: 2, width: "auto", px: 1 }}
                aria-label="Sign out"
              >
                <ListItemIcon sx={{ minWidth: 0, color: "inherit" }}>
                  <LogoutIcon fontSize="small" />
                </ListItemIcon>
              </ListItemButton>
            </Stack>
          </Stack>
        </Stack>
      </Drawer>

      <Box component="main" sx={{ flexGrow: 1, minWidth: 0, p: 4 }}>
        <Stack spacing={3}>
          {message ? (
            <Alert severity={message.severity} onClose={() => setMessage(null)}>
              {message.text}
            </Alert>
          ) : null}
          <Outlet />
        </Stack>
      </Box>
    </Box>
  );
}
