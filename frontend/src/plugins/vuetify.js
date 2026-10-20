import 'vuetify/styles';

import { h } from 'vue';
import { createVuetify } from 'vuetify';
import { aliases, mdi } from 'vuetify/iconsets/mdi-svg';
import iconPaths from './icons';

// Theme choice ('system' follows the OS setting), remembered per browser.
const THEME_KEY = 'listpocket.theme';
export const THEME_CHOICES = ['system', 'light', 'dark'];

export function storedThemeChoice() {
  try {
    const v = localStorage.getItem(THEME_KEY);
    return THEME_CHOICES.includes(v) ? v : 'system';
  } catch {
    return 'system';
  }
}

export function saveThemeChoice(choice) {
  try {
    localStorage.setItem(THEME_KEY, choice);
  } catch {
    // Storage unavailable (private mode); the choice lasts for this session.
  }
}

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
    defaultTheme: storedThemeChoice(),
    themes: {
      light: {
        colors: {
          primary: '#0f4c81',
          secondary: '#557086',
          background: '#f6f7fb',
          surface: '#ffffff',
          'surface-light': '#f4f7fc',
          error: '#b3261e',
          success: '#1b7f4a',
          warning: '#9a6700',
        },
      },
      dark: {
        dark: true,
        colors: {
          primary: '#8cb8e8',
          secondary: '#a3b6c6',
          background: '#0f1318',
          surface: '#171c23',
          'surface-light': '#212833',
          error: '#ffb4ab',
          success: '#7dd3a0',
          warning: '#f0c060',
          info: '#7cc4fa',
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
