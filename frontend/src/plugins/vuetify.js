import 'vuetify/styles';

import { h } from 'vue';
import { createVuetify } from 'vuetify';
import { aliases, mdi } from 'vuetify/iconsets/mdi-svg';
import iconPaths from './icons';

// Resolve `mdi-*` names to bundled SVG paths instead of loading the full
// Material Design Icons webfont. Vuetify's own aliases are already SVG paths.
const mdiNamed = {
  component: (props) => {
    const path = iconPaths[props.icon];
    if (!path && import.meta.env.DEV && String(props.icon).startsWith('mdi-')) {
      console.warn(`Icon "${props.icon}" is not in src/plugins/icons.js`);
    }
    return h(mdi.component, { ...props, icon: path || props.icon });
  },
};

export default createVuetify({
  icons: {
    defaultSet: 'mdi',
    aliases,
    sets: {
      mdi: mdiNamed,
    },
  },
  theme: {
    defaultTheme: 'light',
    themes: {
      light: {
        colors: {
          primary: '#0f4c81',
          secondary: '#557086',
          background: '#f6f7fb',
          surface: '#ffffff',
          error: '#b3261e',
          success: '#1b7f4a',
          warning: '#9a6700',
        },
      },
    },
  },
  defaults: {
    global: {
      density: 'comfortable',
    },
    VCard: {
      rounded: 'xl',
      elevation: 0,
    },
    VBtn: {
      rounded: 'lg',
      variant: 'outlined',
      style: 'text-transform: none; letter-spacing: normal;',
    },
    VTextField: {
      variant: 'outlined',
      density: 'comfortable',
    },
    VTextarea: {
      variant: 'outlined',
      density: 'comfortable',
    },
    VSelect: {
      variant: 'outlined',
      density: 'comfortable',
    },
    VCombobox: {
      variant: 'outlined',
      density: 'comfortable',
    },
    VAutocomplete: {
      variant: 'outlined',
      density: 'comfortable',
    },
    VSheet: {
      rounded: 'lg',
    },
    VChip: {
      rounded: 'lg',
      size: 'small',
    },
  },
});
