import * as React from 'react';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import {useMediaQuery} from "@mui/material";
import theme from "./theme";
import DialogFooter from "./DialogFooter";

const UpgradeDialog = (props) => {
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));

    const handleSuccess = async () => {
        // TODO
    }

    return (
        <Dialog open={props.open} onClose={props.onCancel} fullScreen={fullScreen}>
            <DialogTitle>Upgrade to Pro</DialogTitle>
            <DialogContent>
                Content
            </DialogContent>
            <DialogFooter>
                Footer
            </DialogFooter>
        </Dialog>
    );
};

export default UpgradeDialog;
