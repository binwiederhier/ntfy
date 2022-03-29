import * as React from "react";
import Box from "@mui/material/Box";
import DialogContentText from "@mui/material/DialogContentText";
import DialogActions from "@mui/material/DialogActions";

const DialogFooter = (props) => {
    return (
        <Box sx={{
            display: 'flex',
            flexDirection: 'row',
            justifyContent: 'space-between',
            paddingLeft: '24px',
            paddingTop: '8px 24px',
            paddingBottom: '8px 24px',
        }}>
            <DialogContentText sx={{
                margin: '0px',
                paddingTop: '8px',
            }}>
                {props.status}
            </DialogContentText>
            <DialogActions>
                {props.children}
            </DialogActions>
        </Box>
    );
};

export default DialogFooter;
