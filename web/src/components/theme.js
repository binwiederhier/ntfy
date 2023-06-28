/** @type {import("@mui/material").ThemeOptions} */
const themeOptions = {
  components: {
    MuiListItemIcon: {
      styleOverrides: {
        root: {
          minWidth: "36px",
        },
      },
    },
    MuiCardContent: {
      styleOverrides: {
        root: {
          ":last-child": {
            paddingBottom: "16px",
          },
        },
      },
    },
  },
};

// https://github.com/binwiederhier/ntfy-android/blob/main/app/src/main/res/values/colors.xml

/** @type {import("@mui/material").ThemeOptions['palette']} */
export const lightPalette = {
  mode: "light",
  primary: {
    main: "#338574",
  },
  secondary: {
    main: "#6cead0",
  },
  error: {
    main: "#c30000",
  },
};

/** @type {import("@mui/material").ThemeOptions['palette']} */
export const darkPalette = {
  mode: "dark",
  background: {
    paper: "#1b2124",
  },
  primary: {
    main: "#65b5a3",
  },
  secondary: {
    main: "#6cead0",
  },
  error: {
    main: "#fe4d2e",
  },
};

export default themeOptions;
