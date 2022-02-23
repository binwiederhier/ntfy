import * as React from 'react';
import {useEffect, useState} from 'react';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import WsConnection from '../app/WsConnection';
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
import LocalStorage from "../app/Storage";
import Api from "../app/Api";

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
            {Object.keys(subscriptions).map(id =>
                <SubscriptionNavItem
                    key={id}
                    subscription={subscriptions[id]}
                    selected={props.selectedSubscription === subscriptions[id]}
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
    console.log("Launching App component");

    const [drawerOpen, setDrawerOpen] = useState(true);
    const [subscriptions, setSubscriptions] = useState(LocalStorage.getSubscriptions());
    const [connections, setConnections] = useState({});
    const [selectedSubscription, setSelectedSubscription] = useState(null);
    const [subscribeDialogOpen, setSubscribeDialogOpen] = useState(false);
    const subscriptionChanged = (subscription) => {
        setSubscriptions(prev => ({...prev, [subscription.id]: subscription}));
    };
    const handleSubscribeSubmit = (subscription) => {
        const connection = new WsConnection(subscription, subscriptionChanged);
        setSubscribeDialogOpen(false);
        setSubscriptions(prev => ({...prev, [subscription.id]: subscription}));
        setConnections(prev => ({...prev, [subscription.id]: connection}));
        setSelectedSubscription(subscription);
        Api.poll(subscription.baseUrl, subscription.topic)
            .then(messages => {
                messages.forEach(m => subscription.addNotification(m));
                setSubscriptions(prev => ({...prev, [subscription.id]: subscription}));
            });
        connection.start();
    };
    const handleSubscribeCancel = () => {
        console.log(`Cancel clicked`);
        setSubscribeDialogOpen(false);
    };
    const handleUnsubscribe = (subscription) => {
        setSubscriptions(prev => {
            const newSubscriptions = {...prev};
            delete newSubscriptions[subscription.id];
            const newSubscriptionValues = Object.values(newSubscriptions);
            if (newSubscriptionValues.length > 0) {
                setSelectedSubscription(newSubscriptionValues[0]);
            } else {
                setSelectedSubscription(null);
            }
            return newSubscriptions;
        });
    };
    const handleSubscriptionClick = (subscriptionId) => {
        console.log(`Selected subscription ${subscriptionId}`);
        setSelectedSubscription(subscriptions[subscriptionId]);
    };
    const notifications = (selectedSubscription !== null) ? selectedSubscription.notifications : [];
    const toggleDrawer = () => {
        setDrawerOpen(!drawerOpen);
    };
    useEffect(() => {
        console.log("Starting connections");
        Object.keys(subscriptions).forEach(topicUrl => {
            console.log(`Starting connection for ${topicUrl}`);
            const subscription = subscriptions[topicUrl];
            const connection = new WsConnection(subscription, subscriptionChanged);
            connection.start();
        });
        return () => {
            console.log("Stopping connections");
            Object.keys(connections).forEach(topicUrl => {
                console.log(`Stopping connection for ${topicUrl}`);
                const connection = connections[topicUrl];
                connection.cancel();
            });
        };
    }, [/* only on initial render */]);
    useEffect(() => {
        console.log(`Saving subscriptions`);
        LocalStorage.saveSubscriptions(subscriptions);
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
