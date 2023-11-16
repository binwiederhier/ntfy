import { Typography, Container, Backdrop, styled } from "@mui/material";

export const Paragraph = styled(Typography)({
  paddingTop: 8,
  paddingBottom: 8,
});

export const VerticallyCenteredContainer = styled(Container)(({ theme }) => ({
  display: "flex",
  flexGrow: 1,
  flexDirection: "column",
  justifyContent: "center",
  alignContent: "center",
  color: theme.palette.text.primary,
}));

export const LightboxBackdrop = styled(Backdrop)({
  backgroundColor: "rgba(0, 0, 0, 0.8)", // was: 0.5
});
