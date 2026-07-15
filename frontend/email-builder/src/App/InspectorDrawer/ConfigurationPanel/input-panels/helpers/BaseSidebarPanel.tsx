import React from 'react';

import { Box, Stack, Typography } from '@mui/material';

type SidebarPanelProps = {
  title: string;
  children: React.ReactNode;
};
export default function BaseSidebarPanel({ title, children }: SidebarPanelProps) {
  return (
    <Box px={2} pt={2.5} pb={2}>
      <Typography variant="overline" color="text.secondary" sx={{ display: 'block', mb: 1.5 }}>
        {title}
      </Typography>
      <Stack spacing={3} mb={2}>
        {children}
      </Stack>
    </Box>
  );
}
