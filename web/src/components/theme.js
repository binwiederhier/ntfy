import { red } from '@mui/material/colors';
import { createTheme } from '@mui/material/styles';

const theme = createTheme({
  palette: {
    primary: {
      main: '#338574',
    },
    secondary: {
      main: '#6cead0',
    },
    error: {
      main: red.A400,
    },
  },
  components: {
    MuiListItemIcon: {
      styleOverrides: {
        root: {
          minWidth: '36px',
        },
      },
    },
    MuiBackdrop: {
      styleOverrides: {
        root: {
          backgroundColor: 'rgba(0, 0, 0, 0.8)' // was: 0.5
        }
      }
    }
  },
});

export default theme;
