import * as React from "react";
import { Avatar, Box } from "@mui/material";
import logo from "../img/ntfy-filled.svg";

const AvatarBox = (props) => (
  <Box
    sx={{
      display: "flex",
      flexGrow: 1,
      justifyContent: "center",
      flexDirection: "column",
      alignContent: "center",
      alignItems: "center",
      height: "100vh",
    }}
  >
    <Avatar sx={{ m: 2, width: 64, height: 64, borderRadius: 3 }} src={logo} variant="rounded" />
    {props.children}
  </Box>
);

export default AvatarBox;
