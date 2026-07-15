import React from 'react';

import {
  Box, Drawer, Tab, Tabs,
} from '@mui/material';

import { setSidebarTab, useInspectorDrawerOpen, useSelectedSidebarTab } from '../../documents/editor/EditorContext';

import ConfigurationPanel from './ConfigurationPanel';
import StylesPanel from './StylesPanel';

export const INSPECTOR_DRAWER_WIDTH = 320;

export default function InspectorDrawer() {
  const selectedSidebarTab = useSelectedSidebarTab();
  const inspectorDrawerOpen = useInspectorDrawerOpen();

  const renderCurrentSidebarPanel = () => {
    switch (selectedSidebarTab) {
      case 'block-configuration':
        return <ConfigurationPanel />;
      case 'styles':
        return <StylesPanel />;
    }
  };

  return (
    <Drawer
      variant="persistent"
      anchor="right"
      className="sidebar"
      open={inspectorDrawerOpen}
      sx={{
        width: inspectorDrawerOpen ? INSPECTOR_DRAWER_WIDTH : 0,
      }}
      slotProps={{
        paper: {
          style: {
            position: 'absolute',
            zIndex: 0,
            height: '100%',
            display: 'flex',
            flexDirection: 'column',
          },
        },
        modal: {
          container: document.querySelector('.email-builder-container'),
          style: { position: 'absolute', zIndex: 0 },
        },
      }}
    >
      <Box sx={{
        width: INSPECTOR_DRAWER_WIDTH,
        height: 49,
        flexShrink: 0,
        borderBottom: 1,
        borderColor: 'divider',
        display: 'flex',
        alignItems: 'center',
      }}
      >
        <Box px={2} width="100%">
          <Tabs
            value={selectedSidebarTab}
            onChange={(_, v) => setSidebarTab(v)}
            sx={{ minHeight: 49, '& .MuiTabs-flexContainer': { height: 49 } }}
          >
            <Tab value="styles" label="Styles" />
            <Tab value="block-configuration" label="Inspect" />
          </Tabs>
        </Box>
      </Box>
      <Box sx={{ width: INSPECTOR_DRAWER_WIDTH, flex: 1, minHeight: 0, overflow: 'auto' }}>
        {renderCurrentSidebarPanel()}
      </Box>
    </Drawer>
  );
}
