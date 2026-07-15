declare module '@mui/material/styles' {
  interface Palette {
    brand: {
      navy: string;
      blue: string;
      red: string;
      green: string;
      yellow: string;
      purple: string;
      brown: string;
    };
    cadet: {
      100: string;
      200: string;
      300: string;
      400: string;
      500: string;
    };
    highlight: {
      100: string;
      200: string;
      300: string;
      400: string;
      500: string;
    };
  }
  interface PaletteOptions {
    brand?: Palette['brand'];
    cadet?: Palette['cadet'];
    highlight?: Palette['highlight'];
  }
}

export {};
