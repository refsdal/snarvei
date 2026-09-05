import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import { Alert, IconButton, Stack, Typography } from "@mui/material";
import { useMemo, useState } from "react";

export function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  const copiedLabel = useMemo(
    () =>
      copied ? (
        <Alert severity="success" sx={{ py: 0 }}>
          Copied
        </Alert>
      ) : null,
    [copied],
  );

  return (
    <Stack direction="row" spacing={1} sx={{ alignItems: "center", minWidth: 0 }}>
      <Typography variant="body2" sx={{ fontFamily: "monospace", overflow: "hidden", textOverflow: "ellipsis" }}>
        {value}
      </Typography>
      <IconButton
        size="small"
        aria-label={`Copy ${value}`}
        onClick={async (event) => {
          event.stopPropagation();
          try {
            await navigator.clipboard.writeText(value);
            setCopied(true);
            window.setTimeout(() => setCopied(false), 1200);
          } catch {
            // Clipboard access can be denied (insecure context / permissions); the value stays visible to copy manually.
          }
        }}
      >
        <ContentCopyIcon fontSize="inherit" />
      </IconButton>
      {copiedLabel}
    </Stack>
  );
}
