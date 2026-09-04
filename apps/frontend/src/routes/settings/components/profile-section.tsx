import AddPhotoAlternateOutlinedIcon from "@mui/icons-material/AddPhotoAlternateOutlined";
import { Avatar, Button, Chip, Stack, TextField, Typography } from "@mui/material";
import { useRef } from "react";
import { authClient } from "../../../lib/auth-client";
import type { SessionData } from "../../../types";
import { SectionCard } from "./section-card";
import type { SharedSectionProps } from "./types";

export function ProfileSection(
  props: SharedSectionProps & {
    profileName: string;
    session: SessionData;
    setProfileName: (value: string) => void;
  },
) {
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  return (
    <SectionCard
      title="Profile"
      description="Update your display name and private profile image."
      icon={<AddPhotoAlternateOutlinedIcon />}
    >
      <Stack direction={{ xs: "column", md: "row" }} spacing={3} sx={{ alignItems: { md: "center" } }}>
        <Stack spacing={1.5} sx={{ alignItems: "center", minWidth: 160 }}>
          <Avatar
            src={props.session.user.image ?? undefined}
            sx={{ width: 88, height: 88, bgcolor: "primary.main", fontSize: 34 }}
          >
            {props.session.user.name[0]?.toUpperCase() ?? "U"}
          </Avatar>
          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              onClick={() => fileInputRef.current?.click()}
              disabled={props.busyAction === "upload-image"}
            >
              Upload photo
            </Button>
            <Button
              variant="text"
              color="inherit"
              disabled={!props.session.user.image || props.busyAction === "remove-image"}
              onClick={() =>
                void props.runAction("remove-image", async () => {
                  const response = await fetch("/api/me/profile-image", {
                    method: "DELETE",
                    credentials: "include",
                  });
                  if (!response.ok) {
                    props.setMessage({ severity: "error", text: "Unable to remove profile image." });
                    return;
                  }
                  await props.refreshSessionState();
                  props.setMessage({ severity: "success", text: "Profile image removed." });
                })
              }
            >
              Remove
            </Button>
          </Stack>
          <input
            ref={fileInputRef}
            hidden
            type="file"
            accept="image/png,image/jpeg,image/webp,image/gif"
            onChange={(event) => {
              const file = event.target.files?.[0];
              event.target.value = "";
              if (!file) {
                return;
              }

              void props.runAction("upload-image", async () => {
                const formData = new FormData();
                formData.append("file", file);
                const response = await fetch("/api/me/profile-image", {
                  method: "POST",
                  credentials: "include",
                  body: formData,
                });
                if (!response.ok) {
                  props.setMessage({ severity: "error", text: "Unable to upload profile image." });
                  return;
                }
                await props.refreshSessionState();
                props.setMessage({ severity: "success", text: "Profile image updated." });
              });
            }}
          />
        </Stack>

        <Stack spacing={2} sx={{ flex: 1 }}>
          <TextField
            label="Display name"
            value={props.profileName}
            onChange={(event) => props.setProfileName(event.target.value)}
            slotProps={{ htmlInput: { "data-testid": "settings-name-input" } }}
          />
          <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5} sx={{ alignItems: { sm: "center" } }}>
            <Chip
              label={props.session.user.emailVerified ? "Email verified" : "Email not verified"}
              color={props.session.user.emailVerified ? "success" : "warning"}
            />
            <Typography color="text.secondary">Current email: {props.session.user.email}</Typography>
          </Stack>
          <Button
            variant="contained"
            sx={{ alignSelf: "flex-start" }}
            disabled={
              !props.profileName.trim() ||
              props.profileName.trim() === props.session.user.name ||
              props.busyAction === "save-profile"
            }
            onClick={() =>
              void props.runAction("save-profile", async () => {
                const result = await authClient.updateUser({ name: props.profileName.trim() });
                if (result.error) {
                  props.setMessage({
                    severity: "error",
                    text: result.error.message ?? "Unable to update your profile.",
                  });
                  return;
                }
                await props.refreshSessionState();
                props.setMessage({ severity: "success", text: "Profile updated." });
              })
            }
          >
            Save profile
          </Button>
        </Stack>
      </Stack>
    </SectionCard>
  );
}
