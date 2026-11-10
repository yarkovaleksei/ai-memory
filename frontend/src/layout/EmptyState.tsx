import { Box, Stack, Typography, type SvgIconProps } from "@mui/material";
import type { ComponentType, ReactNode } from "react";

interface EmptyStateProps {
  icon: ComponentType<SvgIconProps>;
  title: string;
  hint?: string;
  action?: ReactNode;
}

// EmptyState — заглушка пустого места: крупная иконка, заголовок и подсказка
// по центру свободной области, а не строка в левом верхнем углу.
export const EmptyState = ({ icon: Icon, title, hint, action }: EmptyStateProps) => (
  <Box
    sx={{
      height: "100%",
      minHeight: 240,
      display: "flex",
      alignItems: "center",
      justifyContent: "center",
      p: 3,
    }}
  >
    <Stack spacing={1} sx={{ alignItems: "center", textAlign: "center", maxWidth: 360 }}>
      <Box
        sx={{
          width: 72,
          height: 72,
          borderRadius: "50%",
          bgcolor: "action.hover",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          mb: 1,
        }}
      >
        <Icon sx={{ fontSize: 40, color: "text.secondary" }} />
      </Box>
      <Typography variant="h6" component="p">
        {title}
      </Typography>
      {hint && (
        <Typography variant="body2" color="text.secondary">
          {hint}
        </Typography>
      )}
      {action}
    </Stack>
  </Box>
);
