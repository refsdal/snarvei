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
import { Link, Outlet, useLocation, useNavigate, useParams } from "@tanstack/react-router";
import { signOut } from "../lib/auth-client";
import type { Organization } from "../lib/data";
import { useMe, useOrganizations } from "../lib/data";
import { getOrganizationPathSegment, orgParams, settingsPath } from "../lib/routes";
import { useMessage } from "./message-context";

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
  const { org } = useParams({ strict: false });
  const { data: me } = useMe();
  const { data: organizations = [] } = useOrganizations();
  const { message, setMessage } = useMessage();
  const activeOrganizationId = me?.session.activeOrganizationId ?? null;
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
                  const nextOrganization = organizations.find((organization) => organization.id === organizationId);
                  void navigate({ to: "/app/$org/dashboard", params: orgParams(nextOrganization) });
                }
              }}
            >
              {organizations.map((organization: Organization) => (
                <MenuItem key={organization.id} value={organization.id}>
                  {organization.name}
                </MenuItem>
              ))}
            </Select>
            <Stack direction="row" spacing={1.5} sx={{ alignItems: "center", pt: 1 }}>
              <Avatar src={me?.user.image ?? undefined} sx={{ bgcolor: "primary.main" }}>
                {me?.user.name?.[0]?.toUpperCase() ?? "S"}
              </Avatar>
              <Box sx={{ minWidth: 0, flexGrow: 1 }}>
                <Typography sx={{ fontWeight: 700 }} noWrap>
                  {me?.user.name ?? "Signed in user"}
                </Typography>
                <Typography variant="body2" color="text.secondary" noWrap>
                  {me?.user.email ?? ""}
                </Typography>
              </Box>
              <ListItemButton
                onClick={() => void signOut().then(() => navigate({ to: "/" }))}
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
