import AddPhotoAlternateOutlinedIcon from "@mui/icons-material/AddPhotoAlternateOutlined";
import { Avatar, Button, Stack, TextField, Typography } from "@mui/material";
import { useRef } from "react";
import { useDeleteProfileImage, useUpdateMe, useUploadProfileImage } from "../../../lib/data";
import { SectionCard } from "./section-card";
import type { SharedSectionProps } from "./types";

export function ProfileSection(
  props: SharedSectionProps & {
    profileName: string;
    setProfileName: (value: string) => void;
  },
) {
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const updateMe = useUpdateMe();
  const uploadProfileImage = useUploadProfileImage();
  const deleteProfileImage = useDeleteProfileImage();

  return (
    <SectionCard
      title="Profile"
      description="Update your display name and private profile image."
      icon={<AddPhotoAlternateOutlinedIcon />}
    >
      <Stack direction={{ xs: "column", md: "row" }} spacing={3} sx={{ alignItems: { md: "center" } }}>
        <Stack spacing={1.5} sx={{ alignItems: "center", minWidth: 160 }}>
          <Avatar
            src={props.me.user.image ?? undefined}
            sx={{ width: 88, height: 88, bgcolor: "primary.main", fontSize: 34 }}
          >
            {props.me.user.name[0]?.toUpperCase() ?? "U"}
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
              disabled={!props.me.user.image || props.busyAction === "remove-image"}
              onClick={() =>
                void props.runAction("remove-image", async () => {
                  await deleteProfileImage.mutateAsync();
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
                await uploadProfileImage.mutateAsync(file);
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
          <Typography color="text.secondary">Current email: {props.me.user.email}</Typography>
          <Button
            variant="contained"
            sx={{ alignSelf: "flex-start" }}
            disabled={
              !props.profileName.trim() ||
              props.profileName.trim() === props.me.user.name ||
              props.busyAction === "save-profile"
            }
            onClick={() =>
              void props.runAction("save-profile", async () => {
                await updateMe.mutateAsync({ name: props.profileName.trim() });
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
