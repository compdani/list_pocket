import 'vuetify/styles';
import '@mdi/font/css/materialdesignicons.css';

import { createVuetify } from 'vuetify';

export default createVuetify({
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
    VCard: {
      rounded: 'xl',
    },
    VBtn: {
      rounded: 'lg',
    },
    VTextField: {
      variant: 'outlined',
      density: 'comfortable',
    },
  },
});
