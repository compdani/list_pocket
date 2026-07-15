import React, { useState } from 'react';

import { Stack, ToggleButtonGroup } from '@mui/material';

import SidebarFieldLabel from './SidebarFieldLabel';

type Props = {
  label: string | JSX.Element;
  children: JSX.Element | JSX.Element[];
  defaultValue: string;
  onChange: (v: string) => void;
};
export default function RadioGroupInput({ label, children, defaultValue, onChange }: Props) {
  const [value, setValue] = useState(defaultValue);
  return (
    <Stack alignItems="flex-start" spacing={1} width="100%">
      <SidebarFieldLabel>{label}</SidebarFieldLabel>
      <ToggleButtonGroup
        exclusive
        fullWidth
        value={value}
        size="small"
        onChange={(_, v: unknown) => {
          if (typeof v !== 'string') {
            throw new Error('RadioGroupInput can only receive string values');
          }
          setValue(v);
          onChange(v);
        }}
      >
        {children}
      </ToggleButtonGroup>
    </Stack>
  );
}
