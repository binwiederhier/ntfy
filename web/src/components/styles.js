import {styled} from "@mui/styles";
import Typography from "@mui/material/Typography";
import theme from "./theme";
import Container from "@mui/material/Container";

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
