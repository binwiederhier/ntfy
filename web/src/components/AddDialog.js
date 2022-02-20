import * as React from 'react';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import {useState} from "react";
import Subscription from "../app/Subscription";

const defaultBaseUrl = "https://ntfy.sh"

const AddDialog = (props) => {
    const [topic, setTopic] = useState("");
    const handleCancel = () => {
        setTopic('');
        props.onCancel();
    }
    const handleSubmit = () => {
        const subscription = new Subscription(defaultBaseUrl, topic);
        props.onSubmit(subscription);
        setTopic('');
    }
    return (
        <>
            <Dialog open={props.open} onClose={props.onClose}>
                <DialogTitle>Subscribe to topic</DialogTitle>
                <DialogContent>
                    <DialogContentText>
                        Topics may not be password-protected, so choose a name that's not easy to guess.
                        Once subscribed, you can PUT/POST notifications.
                    </DialogContentText>
                    <TextField
                        autoFocus
                        margin="dense"
                        id="name"
                        label="Topic name, e.g. phil_alerts"
                        value={topic}
                        onChange={ev => setTopic(ev.target.value)}
                        type="text"
                        fullWidth
                        variant="standard"
                    />
                </DialogContent>
                <DialogActions>
                    <Button onClick={handleCancel}>Cancel</Button>
                    <Button onClick={handleSubmit} autoFocus disabled={topic === ""}>Subscribe</Button>
                </DialogActions>
            </Dialog>
        </>
    );
};

export default AddDialog;
