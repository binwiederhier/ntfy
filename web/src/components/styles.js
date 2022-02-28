import {makeStyles, styled} from "@mui/styles";
import Typography from "@mui/material/Typography";
import theme from "./theme";
import Container from "@mui/material/Container";

const useStyles = makeStyles(theme => ({
  bottomBar: {
    display: 'flex',
    flexDirection: 'row',
    justifyContent: 'space-between',
    paddingLeft: '24px',
    paddingTop: '8px 24px',
    paddingBottom: '8px 24px',
  },
  statusText: {
    margin: '0px',
    paddingTop: '8px',
  }
}));

export const Paragraph = styled(Typography)({
  paddingTop: 8,
  paddingBottom: 8,
});

export const VerticallyCenteredContainer = styled(Container)({
  display: 'flex',
  flexGrow: 1,
  flexDirection: 'column',
  justifyContent: 'center',
  alignContent: 'center',
  color: theme.palette.body.main
});

export default useStyles;
