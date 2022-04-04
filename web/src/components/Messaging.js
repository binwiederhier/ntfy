import * as React from 'react';
import {useState} from 'react';
import Navigation from "./Navigation";
import {topicUrl} from "../app/utils";
import Paper from "@mui/material/Paper";
import IconButton from "@mui/material/IconButton";
import TextField from "@mui/material/TextField";
import SendIcon from "@mui/icons-material/Send";
import api from "../app/Api";
import SendDialog from "./SendDialog";
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import EmojiPicker from "./EmojiPicker";

const Messaging = (props) => {
    const [message, setMessage] = useState("");
    const [dialogKey, setDialogKey] = useState(0);

    const dialogOpenMode = props.dialogOpenMode;
    const subscription = props.selected;
    const selectedTopicUrl = (subscription) ? topicUrl(subscription.baseUrl, subscription.topic) : "";

    const handleOpenDialogClick = () => {
        props.onDialogOpenModeChange(SendDialog.OPEN_MODE_DEFAULT);
    };

    const handleSendDialogClose = () => {
        props.onDialogOpenModeChange("");
        setDialogKey(prev => prev+1);
    };

    return (
        <>
            {subscription && <MessageBar
                subscription={subscription}
                message={message}
                onMessageChange={setMessage}
                onOpenDialogClick={handleOpenDialogClick}
            />}
            <SendDialog
                key={`sendDialog${dialogKey}`} // Resets dialog when canceled/closed
                openMode={dialogOpenMode}
                topicUrl={selectedTopicUrl}
                message={message}
                onClose={handleSendDialogClose}
                onDragEnter={() => props.onDialogOpenModeChange(prev => (prev) ? prev : SendDialog.OPEN_MODE_DRAG)} // Only update if not already open
                onResetOpenMode={() => props.onDialogOpenModeChange(SendDialog.OPEN_MODE_DEFAULT)}
            />
        </>
    );
}

const MessageBar = (props) => {
    const subscription = props.subscription;
    const handleSendClick = () => {
        api.publish(subscription.baseUrl, subscription.topic, props.message); // FIXME
        props.onMessageChange("");
    };
    return (
        <Paper
            elevation={3}
            sx={{
                display: "flex",
                position: 'fixed',
                bottom: 0,
                right: 0,
                padding: 2,
                width: `calc(100% - ${Navigation.width}px)`,
                backgroundColor: (theme) => theme.palette.mode === 'light' ? theme.palette.grey[100] : theme.palette.grey[900]
            }}
        >
            <IconButton color="inherit" size="large" edge="start" onClick={props.onOpenDialogClick}>
                <KeyboardArrowUpIcon/>
            </IconButton>
            <TextField
                autoFocus
                margin="dense"
                placeholder="Message"
                type="text"
                fullWidth
                variant="standard"
                value={props.message}
                onChange={ev => props.onMessageChange(ev.target.value)}
                onKeyPress={(ev) => {
                    if (ev.key === 'Enter') {
                        ev.preventDefault();
                        handleSendClick();
                    }
                }}
            />
            <IconButton color="inherit" size="large" edge="end" onClick={handleSendClick}>
                <SendIcon/>
            </IconButton>
        </Paper>
    );
};

export default Messaging;
