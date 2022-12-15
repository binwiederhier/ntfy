import AppBar from "@mui/material/AppBar";
import Navigation from "./Navigation";
import Toolbar from "@mui/material/Toolbar";
import IconButton from "@mui/material/IconButton";
import MenuIcon from "@mui/icons-material/Menu";
import Typography from "@mui/material/Typography";
import * as React from "react";
import {useEffect, useRef, useState} from "react";
import Box from "@mui/material/Box";
import {formatShortDateTime, shuffle, topicDisplayName, topicShortUrl} from "../app/utils";
import {useLocation, useNavigate} from "react-router-dom";
import ClickAwayListener from '@mui/material/ClickAwayListener';
import Grow from '@mui/material/Grow';
import Paper from '@mui/material/Paper';
import Popper from '@mui/material/Popper';
import MenuItem from '@mui/material/MenuItem';
import MenuList from '@mui/material/MenuList';
import MoreVertIcon from "@mui/icons-material/MoreVert";
import NotificationsIcon from '@mui/icons-material/Notifications';
import NotificationsOffIcon from '@mui/icons-material/NotificationsOff';
import api from "../app/Api";
import routes from "./routes";
import subscriptionManager from "../app/SubscriptionManager";
import logo from "../img/ntfy.svg";
import {useTranslation} from "react-i18next";
import {Portal, Snackbar} from "@mui/material";
import SubscriptionSettingsDialog from "./SubscriptionSettingsDialog";
import session from "../app/Session";
import AccountCircleIcon from '@mui/icons-material/AccountCircle';
import Button from "@mui/material/Button";

const ActionBar = (props) => {
    const { t } = useTranslation();
    const location = useLocation();
    let title = "ntfy";
    if (props.selected) {
        title = topicDisplayName(props.selected);
    } else if (location.pathname === "/settings") {
        title = t("action_bar_settings");
    }
    return (
        <AppBar position="fixed" sx={{
            width: '100%',
            zIndex: { sm: 1250 }, // > Navigation (1200), but < Dialog (1300)
            ml: { sm: `${Navigation.width}px` }
        }}>
            <Toolbar sx={{pr: '24px'}}>
                <IconButton
                    color="inherit"
                    edge="start"
                    aria-label={t("action_bar_show_menu")}
                    onClick={props.onMobileDrawerToggle}
                    sx={{ mr: 2, display: { sm: 'none' } }}
                >
                    <MenuIcon />
                </IconButton>
                <Box
                    component="img"
                    src={logo}
                    alt={t("action_bar_logo_alt")}
                    sx={{
                        display: { xs: 'none', sm: 'block' },
                        marginRight: '10px',
                        height: '28px'
                    }}
                />
                <Typography variant="h6" noWrap component="div" sx={{ flexGrow: 1 }}>
                    {title}
                </Typography>
                {props.selected &&
                    <SettingsIcons
                        subscription={props.selected}
                        onUnsubscribe={props.onUnsubscribe}
                    />}
                <ProfileIcon/>
            </Toolbar>
        </AppBar>
    );
};

// Originally from https://mui.com/components/menus/#MenuListComposition.js
const SettingsIcons = (props) => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const [open, setOpen] = useState(false);
    const [snackOpen, setSnackOpen] = useState(false);
    const [subscriptionSettingsOpen, setSubscriptionSettingsOpen] = useState(false);
    const anchorRef = useRef(null);
    const subscription = props.subscription;

    const handleToggleOpen = () => {
        setOpen((prevOpen) => !prevOpen);
    };

    const handleToggleMute = async () => {
        const mutedUntil = (subscription.mutedUntil) ? 0 : 1; // Make this a timestamp in the future
        await subscriptionManager.setMutedUntil(subscription.id, mutedUntil);
    }

    const handleClose = (event) => {
        if (anchorRef.current && anchorRef.current.contains(event.target)) {
            return;
        }
        setOpen(false);
    };

    const handleClearAll = async (event) => {
        handleClose(event);
        console.log(`[ActionBar] Deleting all notifications from ${props.subscription.id}`);
        await subscriptionManager.deleteNotifications(props.subscription.id);
    };

    const handleUnsubscribe = async (event) => {
        console.log(`[ActionBar] Unsubscribing from ${props.subscription.id}`, props.subscription);
        handleClose(event);
        await subscriptionManager.remove(props.subscription.id);
        if (session.exists() && props.subscription.remoteId) {
            await api.deleteAccountSubscription("http://localhost:2586", session.token(), props.subscription.remoteId);
        }
        const newSelected = await subscriptionManager.first(); // May be undefined
        if (newSelected) {
            navigate(routes.forSubscription(newSelected));
        } else {
            navigate(routes.app);
        }
    };

    const handleSubscriptionSettings = async () => {
        setSubscriptionSettingsOpen(true);
    }

    const handleSendTestMessage = async () => {
        const baseUrl = props.subscription.baseUrl;
        const topic = props.subscription.topic;
        const tags = shuffle([
            "grinning", "octopus", "upside_down_face", "palm_tree", "maple_leaf", "apple", "skull", "warning", "jack_o_lantern",
            "de-server-1", "backups", "cron-script", "script-error", "phils-automation", "mouse", "go-rocks", "hi-ben"])
                .slice(0, Math.round(Math.random() * 4));
        const priority = shuffle([1, 2, 3, 4, 5])[0];
        const title = shuffle([
            "",
            "",
            "", // Higher chance of no title
            "Oh my, another test message?",
            "Titles are optional, did you know that?",
            "ntfy is open source, and will always be free. Cool, right?",
            "I don't really like apples",
            "My favorite TV show is The Wire. You should watch it!",
            "You can attach files and URLs to messages too",
            "You can delay messages up to 3 days"
        ])[0];
        const nowSeconds = Math.round(Date.now()/1000);
        const message = shuffle([
            `Hello friend, this is a test notification from ntfy web. It's ${formatShortDateTime(nowSeconds)} right now. Is that early or late?`,
            `So I heard you like ntfy? If that's true, go to GitHub and star it, or to the Play store and rate it. Thanks! Oh yeah, this is a test notification.`,
            `It's almost like you want to hear what I have to say. I'm not even a machine. I'm just a sentence that Phil typed on a random Thursday.`,
            `Alright then, it's ${formatShortDateTime(nowSeconds)} already. Boy oh boy, where did the time go? I hope you're alright, friend.`,
            `There are nine million bicycles in Beijing That's a fact; It's a thing we can't deny. I wonder if that's true ...`,
            `I'm really excited that you're trying out ntfy. Did you know that there are a few public topics, such as ntfy.sh/stats and ntfy.sh/announcements.`,
            `It's interesting to hear what people use ntfy for. I've heard people talk about using it for so many cool things. What do you use it for?`
        ])[0];
        try {
            await api.publish(baseUrl, topic, message, {
                title: title,
                priority: priority,
                tags: tags
            });
        } catch (e) {
            console.log(`[ActionBar] Error publishing message`, e);
            setSnackOpen(true);
        }
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
            <IconButton color="inherit" size="large" edge="end" onClick={handleToggleMute} sx={{marginRight: 0}} aria-label={t("action_bar_toggle_mute")}>
                {subscription.mutedUntil ? <NotificationsOffIcon/> : <NotificationsIcon/>}
            </IconButton>
            <IconButton color="inherit" size="large" edge="end" ref={anchorRef} onClick={handleToggleOpen} aria-label={t("action_bar_toggle_action_menu")}>
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
                        style={{transformOrigin: placement === 'bottom-start' ? 'left top' : 'left bottom'}}
                    >
                        <Paper>
                            <ClickAwayListener onClickAway={handleClose}>
                                <MenuList autoFocusItem={open} onKeyDown={handleListKeyDown}>
                                    <MenuItem onClick={handleSubscriptionSettings}>{t("action_bar_subscription_settings")}</MenuItem>
                                    <MenuItem onClick={handleSendTestMessage}>{t("action_bar_send_test_notification")}</MenuItem>
                                    <MenuItem onClick={handleClearAll}>{t("action_bar_clear_notifications")}</MenuItem>
                                    <MenuItem onClick={handleUnsubscribe}>{t("action_bar_unsubscribe")}</MenuItem>
                                </MenuList>
                            </ClickAwayListener>
                        </Paper>
                    </Grow>
                )}
            </Popper>
            <Portal>
                <Snackbar
                    open={snackOpen}
                    autoHideDuration={3000}
                    onClose={() => setSnackOpen(false)}
                    message={t("message_bar_error_publishing")}
                />
            </Portal>
            <Portal>
                <SubscriptionSettingsDialog
                    key={`subscriptionSettingsDialog${subscription.id}`}
                    open={subscriptionSettingsOpen}
                    subscription={subscription}
                    onClose={() => setSubscriptionSettingsOpen(false)}
                />
            </Portal>
        </>
    );
};

const ProfileIcon = (props) => {
    const { t } = useTranslation();
    const [open, setOpen] = useState(false);
    const anchorRef = useRef(null);
    const navigate = useNavigate();

    const handleToggleOpen = () => {
        setOpen((prevOpen) => !prevOpen);
    };

    const handleClose = (event) => {
        if (anchorRef.current && anchorRef.current.contains(event.target)) {
            return;
        }
        setOpen(false);
    };

    const handleListKeyDown = (event) => {
        if (event.key === 'Tab') {
            event.preventDefault();
            setOpen(false);
        } else if (event.key === 'Escape') {
            setOpen(false);
        }
    }

    const handleUpgrade = () => {
        // TODO
    };

    const handleLogout = async () => {
        await api.logout("http://localhost:2586"/*window.location.origin*/, session.token());
        session.reset();
        window.location.href = routes.app;
    };

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
            {session.exists() &&
                <IconButton color="inherit" size="large" edge="end" ref={anchorRef} onClick={handleToggleOpen} sx={{marginRight: 0}} aria-label={t("xxxxxxx")}>
                    <AccountCircleIcon/>
                </IconButton>
            }
            {!session.exists() &&
                <>
                    <Button color="inherit" variant="outlined" onClick={() => navigate(routes.login)}>Sign in</Button>
                    <Button color="inherit" variant="outlined" onClick={() => navigate(routes.signup)}>Sign up</Button>
                </>
            }
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
                        style={{transformOrigin: placement === 'bottom-start' ? 'left top' : 'left bottom'}}
                    >
                        <Paper>
                            <ClickAwayListener onClickAway={handleClose}>
                                <MenuList autoFocusItem={open} onKeyDown={handleListKeyDown}>
                                    <MenuItem onClick={handleUpgrade}>Upgrade</MenuItem>
                                    <MenuItem onClick={handleLogout}>Logout</MenuItem>
                                </MenuList>
                            </ClickAwayListener>
                        </Paper>
                    </Grow>
                )}
            </Popper>
        </>
    );
};


export default ActionBar;
