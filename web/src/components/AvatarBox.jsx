import * as React from "react";
import { Avatar, Box, styled } from "@mui/material";
import logo from "../img/ntfy-filled.svg";

const AvatarBoxContainer = styled(Box)`
  display: flex;
  flex-grow: 1;
  justify-content: center;
  flex-direction: column;
  align-content: center;
  align-items: center;
  height: 100dvh;
  max-width: min(400px, 90dvw);
  margin: auto;
`;
const AvatarBox = (props) => (
  <AvatarBoxContainer>
    <Avatar sx={{ m: 2, width: 64, height: 64, borderRadius: 3 }} src={logo} variant="rounded" />
    {props.children}
  </AvatarBoxContainer>
);

export default AvatarBox;
