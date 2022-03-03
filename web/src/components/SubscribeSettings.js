import * as React from 'react';
import {useEffect, useRef, useState} from 'react';
import ClickAwayListener from '@mui/material/ClickAwayListener';
import Grow from '@mui/material/Grow';
import Paper from '@mui/material/Paper';
import Popper from '@mui/material/Popper';
import MenuItem from '@mui/material/MenuItem';
import MenuList from '@mui/material/MenuList';
import IconButton from "@mui/material/IconButton";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import api from "../app/Api";
import subscriptionManager from "../app/SubscriptionManager";

// Originally from https://mui.com/components/menus/#MenuListComposition.js
const SubscribeSettings = (props) => {
    const [open, setOpen] = useState(false);
    const anchorRef = useRef(null);

    const handleToggle = () => {
        setOpen((prevOpen) => !prevOpen);
    };

    const handleClose = (event) => {
        if (anchorRef.current && anchorRef.current.contains(event.target)) {
            return;
        }
        setOpen(false);
    };

    const handleClearAll = async (event) => {
        handleClose(event);
        console.log(`[IconSubscribeSettings] Deleting all notifications from ${props.subscription.id}`);
        await subscriptionManager.deleteNotifications(props.subscription.id);
    };

    const handleUnsubscribe = async (event) => {
        handleClose(event);
        await subscriptionManager.remove(props.subscription.id);
        props.onUnsubscribe(props.subscription.id);
    };

    const handleSendTestMessage = () => {
        const baseUrl = props.subscription.baseUrl;
        const topic = props.subscription.topic;
        api.publish(baseUrl, topic,
            `This is a test notification sent by the ntfy Web UI at ${new Date().toString()}.`); // FIXME result ignored
        setOpen(false);
    }

    const handleListKeyDown = (event) => {
        if (event.key === 'Tab') {
            event.preventDefault();
            setOpen(false);
        } else if (event.key === 'Escape') {
            setOpen(false);
        }
    }

    // return focus to the button when we transitioned from !open -> open
    const prevOpen = useRef(open);
    useEffect(() => {
        if (prevOpen.current === true && open === false) {
            anchorRef.current.focus();
        }
        prevOpen.current = open;
    }, [open]);

    return (
        <>
            <IconButton
                color="inherit"
                size="large"
                edge="end"
                ref={anchorRef}
                id="composition-button"
                onClick={handleToggle}
            >
                <MoreVertIcon/>
            </IconButton>
            <Popper
                open={open}
                anchorEl={anchorRef.current}
                role={undefined}
                placement="bottom-start"
                transition
                disablePortal
            >
                {({TransitionProps, placement}) => (
                    <Grow
                        {...TransitionProps}
                        style={{
                            transformOrigin:
                                placement === 'bottom-start' ? 'left top' : 'left bottom',
                        }}
                    >
                        <Paper>
                            <ClickAwayListener onClickAway={handleClose}>
                                <MenuList
                                    autoFocusItem={open}
                                    id="composition-menu"
                                    onKeyDown={handleListKeyDown}
                                >
                                    <MenuItem onClick={handleSendTestMessage}>Send test notification</MenuItem>
                                    <MenuItem onClick={handleClearAll}>Clear all notifications</MenuItem>
                                    <MenuItem onClick={handleUnsubscribe}>Unsubscribe</MenuItem>
                                </MenuList>
                            </ClickAwayListener>
                        </Paper>
                    </Grow>
                )}
            </Popper>
        </>
    );
}

export default SubscribeSettings;
