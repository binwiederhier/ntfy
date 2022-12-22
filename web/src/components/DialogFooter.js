import * as React from "react";
import Box from "@mui/material/Box";
import DialogContentText from "@mui/material/DialogContentText";
import DialogActions from "@mui/material/DialogActions";

const DialogFooter = (props) => {
  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "row",
        justifyContent: "space-between",
        paddingLeft: "24px",
        paddingBottom: "8px",
      }}
    >
      <DialogContentText
        component="div"
        aria-live="polite"
        sx={{
          margin: "0px",
          paddingTop: "12px",
          paddingBottom: "4px",
        }}
      >
        {props.status}
      </DialogContentText>
      <DialogActions sx={{ paddingRight: 2 }}>{props.children}</DialogActions>
    </Box>
  );
};

export default DialogFooter;
