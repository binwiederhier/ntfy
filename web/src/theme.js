import { red } from '@mui/material/colors';
import { createTheme } from '@mui/material/styles';

// A custom theme for this app
const theme = createTheme({
  palette: {
    primary: {
      main: '#338574',
    },
    secondary: {
      main: '#338574',
    },
    error: {
      main: red.A400,
    },
  },
});

export default theme;
