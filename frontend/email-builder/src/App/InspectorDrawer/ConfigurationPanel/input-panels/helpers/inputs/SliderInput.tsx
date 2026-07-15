import React, { useState } from 'react';

import { Stack } from '@mui/material';

import SidebarFieldLabel from './SidebarFieldLabel';
import RawSliderInput from './raw/RawSliderInput';

type SliderInputProps = {
  label: string;
  iconLabel: JSX.Element;

  step?: number;
  marks?: boolean;
  units: string;
  min?: number;
  max?: number;

  defaultValue: number;
  onChange: (v: number) => void;
};

export default function SliderInput({ label, defaultValue, onChange, ...props }: SliderInputProps) {
  const [value, setValue] = useState(defaultValue);
  return (
    <Stack spacing={1} alignItems="flex-start" width="100%">
      <SidebarFieldLabel>{label}</SidebarFieldLabel>
      <RawSliderInput
        value={value}
        setValue={(value: number) => {
          setValue(value);
          onChange(value);
        }}
        {...props}
      />
    </Stack>
  );
}
