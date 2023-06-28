import { grey, red } from "@mui/material/colors";

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
    main: red.A400,
  },
};

/** @type {import("@mui/material").ThemeOptions['palette']} */
export const darkPalette = {
  ...lightPalette,
  mode: "dark",
  background: {
    paper: grey["800"],
  },
  primary: {
    main: "#6cead0",
  },
};

export default themeOptions;
