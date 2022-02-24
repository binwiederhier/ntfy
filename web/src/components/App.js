import * as React from 'react';
import {useEffect, useState} from 'react';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import {styled, ThemeProvider} from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import MuiDrawer from '@mui/material/Drawer';
import MuiAppBar from '@mui/material/AppBar';
import Toolbar from '@mui/material/Toolbar';
import ChatBubbleOutlineIcon from '@mui/icons-material/ChatBubbleOutline';
import List from '@mui/material/List';
import Divider from '@mui/material/Divider';
import IconButton from '@mui/material/IconButton';
import MenuIcon from '@mui/icons-material/Menu';
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft';
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText from "@mui/material/ListItemText";
import ListItemButton from "@mui/material/ListItemButton";
import SettingsIcon from "@mui/icons-material/Settings";
import AddIcon from "@mui/icons-material/Add";
import AddDialog from "./AddDialog";
import NotificationList from "./NotificationList";
import DetailSettingsIcon from "./DetailSettingsIcon";
import theme from "./theme";
import api from "../app/Api";
import repository from "../app/Repository";
import connectionManager from "../app/ConnectionManager";
import Subscriptions from "../app/Subscriptions";

const drawerWidth = 240;

const AppBar = styled(MuiAppBar, {
    shouldForwardProp: (prop) => prop !== 'open',
})(({ theme, open }) => ({
    zIndex: theme.zIndex.drawer + 1,
    transition: theme.transitions.create(['width', 'margin'], {
        easing: theme.transitions.easing.sharp,
        duration: theme.transitions.duration.leavingScreen,
    }),
    ...(open && {
        marginLeft: drawerWidth,
        width: `calc(100% - ${drawerWidth}px)`,
        transition: theme.transitions.create(['width', 'margin'], {
            easing: theme.transitions.easing.sharp,
            duration: theme.transitions.duration.enteringScreen,
        }),
    }),
}));

const Drawer = styled(MuiDrawer, { shouldForwardProp: (prop) => prop !== 'open' })(
    ({ theme, open }) => ({
        '& .MuiDrawer-paper': {
            position: 'relative',
            whiteSpace: 'nowrap',
            width: drawerWidth,
            transition: theme.transitions.create('width', {
                easing: theme.transitions.easing.sharp,
                duration: theme.transitions.duration.enteringScreen,
            }),
            boxSizing: 'border-box',
            ...(!open && {
                overflowX: 'hidden',
                transition: theme.transitions.create('width', {
                    easing: theme.transitions.easing.sharp,
                    duration: theme.transitions.duration.leavingScreen,
                }),
                width: theme.spacing(7),
                [theme.breakpoints.up('sm')]: {
                    width: theme.spacing(9),
                },
            }),
        },
    }),
);


const SubscriptionNav = (props) => {
    const subscriptions = props.subscriptions;
    return (
        <>
            {subscriptions.map((id, subscription) =>
                <SubscriptionNavItem
                    key={id}
                    subscription={subscription}
                    selected={props.selectedSubscription && props.selectedSubscription.id === id}
                    onClick={() => props.handleSubscriptionClick(id)}
                />)
            }
        </>
    );
}

const SubscriptionNavItem = (props) => {
    const subscription = props.subscription;
    return (
        <ListItemButton onClick={props.onClick} selected={props.selected}>
            <ListItemIcon><ChatBubbleOutlineIcon /></ListItemIcon>
            <ListItemText primary={subscription.shortUrl()}/>
        </ListItemButton>
    );
}

const App = () => {
    console.log(`[App] Rendering main view`);

    const [drawerOpen, setDrawerOpen] = useState(true);
    const [subscriptions, setSubscriptions] = useState(new Subscriptions());
    const [selectedSubscription, setSelectedSubscription] = useState(null);
    const [subscribeDialogOpen, setSubscribeDialogOpen] = useState(false);
    const handleNotification = (subscriptionId, notification) => {
        setSubscriptions(prev => {
            const newSubscription = prev.get(subscriptionId).addNotification(notification);
            return prev.update(newSubscription).clone();
        });
    };
    const handleSubscribeSubmit = (subscription) => {
        console.log(`[App] New subscription: ${subscription.id}`);
        setSubscribeDialogOpen(false);
        setSubscriptions(prev => prev.add(subscription).clone());
        setSelectedSubscription(subscription);
        api.poll(subscription.baseUrl, subscription.topic)
            .then(messages => {
                setSubscriptions(prev => {
                    const newSubscription = prev.get(subscription.id).addNotifications(messages);
                    return prev.update(newSubscription).clone();
                });
            });
    };
    const handleSubscribeCancel = () => {
        console.log(`[App] Cancel clicked`);
        setSubscribeDialogOpen(false);
    };
    const handleClearAll = (subscriptionId) => {
        console.log(`[App] Deleting all notifications from ${subscriptionId}`);
        setSubscriptions(prev => {
            const newSubscription = prev.get(subscriptionId).deleteAllNotifications();
            return prev.update(newSubscription).clone();
        });
    };
    const handleUnsubscribe = (subscriptionId) => {
        console.log(`[App] Unsubscribing from ${subscriptionId}`);
        setSubscriptions(prev => {
            const newSubscriptions = prev.remove(subscriptionId).clone();
            setSelectedSubscription(newSubscriptions.firstOrNull());
            return newSubscriptions;
        });
    };
    const handleSubscriptionClick = (subscriptionId) => {
        console.log(`[App] Selected ${subscriptionId}`);
        setSelectedSubscription(subscriptions.get(subscriptionId));
    };
    const notifications = (selectedSubscription !== null) ? selectedSubscription.getNotifications() : [];
    const toggleDrawer = () => {
        setDrawerOpen(!drawerOpen);
    };
    useEffect(() => {
        connectionManager.refresh(subscriptions, handleNotification);
        repository.saveSubscriptions(subscriptions);
    }, [subscriptions]);
    return (
        <ThemeProvider theme={theme}>
            <CssBaseline />
            <Box sx={{ display: 'flex' }}>
                <AppBar position="absolute" open={drawerOpen}>
                    <Toolbar sx={{pr: '24px'}} color="primary">
                        <IconButton
                            edge="start"
                            color="inherit"
                            aria-label="open drawer"
                            onClick={toggleDrawer}
                            sx={{
                                marginRight: '36px',
                                ...(drawerOpen && { display: 'none' }),
                            }}
                        >
                            <MenuIcon />
                        </IconButton>
                        <Typography
                            component="h1"
                            variant="h6"
                            color="inherit"
                            noWrap
                            sx={{ flexGrow: 1 }}
                        >
                            {(selectedSubscription !== null) ? selectedSubscription.shortUrl() : "ntfy"}
                        </Typography>
                        {selectedSubscription !== null && <DetailSettingsIcon
                            subscription={selectedSubscription}
                            onClearAll={handleClearAll}
                            onUnsubscribe={handleUnsubscribe}
                        />}
                    </Toolbar>
                </AppBar>
                <Drawer variant="permanent" open={drawerOpen}>
                    <Toolbar
                        sx={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'flex-end',
                            px: [1],
                        }}
                    >
                        <IconButton onClick={toggleDrawer}>
                            <ChevronLeftIcon />
                        </IconButton>
                    </Toolbar>
                    <Divider />
                    <List component="nav">
                        <SubscriptionNav
                            subscriptions={subscriptions}
                            selectedSubscription={selectedSubscription}
                            handleSubscriptionClick={handleSubscriptionClick}
                        />
                        <Divider sx={{ my: 1 }} />
                        <ListItemButton>
                            <ListItemIcon>
                                <SettingsIcon />
                            </ListItemIcon>
                            <ListItemText primary="Settings" />
                        </ListItemButton>
                        <ListItemButton onClick={() => setSubscribeDialogOpen(true)}>
                            <ListItemIcon>
                                <AddIcon />
                            </ListItemIcon>
                            <ListItemText primary="Add subscription" />
                        </ListItemButton>
                    </List>
                </Drawer>
                <Box
                    component="main"
                    sx={{
                        backgroundColor: (theme) =>
                            theme.palette.mode === 'light'
                                ? theme.palette.grey[100]
                                : theme.palette.grey[900],
                        flexGrow: 1,
                        height: '100vh',
                        overflow: 'auto',
                    }}
                >
                    <Toolbar />
                    <NotificationList notifications={notifications}/>
                </Box>
            </Box>
            <AddDialog
                open={subscribeDialogOpen}
                onCancel={handleSubscribeCancel}
                onSubmit={handleSubscribeSubmit}
            />
        </ThemeProvider>
    );
}

export default App;
