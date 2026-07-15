import React from 'react';

import { Typography } from '@mui/material';

type Props = {
  children: React.ReactNode;
};

export default function SidebarFieldLabel({ children }: Props) {
  return (
    <Typography variant="body2" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
      {children}
    </Typography>
  );
}
