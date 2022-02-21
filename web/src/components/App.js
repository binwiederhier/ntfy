import * as React from 'react';
import {useEffect, useState} from 'react';
import Container from '@mui/material/Container';
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
import Badge from '@mui/material/Badge';
import Grid from '@mui/material/Grid';
import MenuIcon from '@mui/icons-material/Menu';
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft';
import NotificationsIcon from '@mui/icons-material/Notifications';
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText from "@mui/material/ListItemText";
import ListItemButton from "@mui/material/ListItemButton";
import SettingsIcon from "@mui/icons-material/Settings";
import AddIcon from "@mui/icons-material/Add";
import Card from "@mui/material/Card";
import {CardContent, Stack} from "@mui/material";
import AddDialog from "./AddDialog";
import theme from "./theme";
import LocalStorage from "../app/Storage";

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

const NotificationList = (props) => {
    return (
        <Stack spacing={3} className="notificationList">
            {props.notifications.map(notification =>
                <NotificationItem key={notification.id} notification={notification}/>)}
        </Stack>
    );
}

const NotificationItem = (props) => {
    const notification = props.notification;
    return (
        <Card sx={{ minWidth: 275 }}>
            <CardContent>
                <Typography sx={{ fontSize: 14 }} color="text.secondary" gutterBottom>
                    {notification.time}
                </Typography>
                {notification.title && <Typography variant="h5" component="div">
                    {notification.title}
                </Typography>}
                <Typography variant="body1">
                    {notification.message}
                </Typography>
            </CardContent>
        </Card>
    );
}

const App = () => {
    console.log("Launching App component");

    const [drawerOpen, setDrawerOpen] = useState(true);
    const [subscriptions, setSubscriptions] = useState(LocalStorage.getSubscriptions());
    const [connections, setConnections] = useState({});
    const [selectedSubscription, setSelectedSubscription] = useState(null);
    const [addDialogOpen, setAddDialogOpen] = useState(false);
    const subscriptionChanged = (subscription) => {
        setSubscriptions(prev => ({...prev, [subscription.id]: subscription})); // Fake-replace
    };
    const handleAddSubmit = (subscription) => {
        const connection = new WsConnection(subscription, subscriptionChanged);
        setAddDialogOpen(false);
        setSubscriptions(prev => ({...prev, [subscription.id]: subscription}));
        setConnections(prev => ({...prev, [subscription.id]: connection}));
        connection.start();
    };
    const handleAddCancel = () => {
        console.log(`Cancel clicked`)
        setAddDialogOpen(false);
    }
    const handleSubscriptionClick = (subscriptionId) => {
        console.log(`Selected subscription ${subscriptionId}`)
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
            <Box sx={{ display: 'flex' }}>
                <CssBaseline />
                <AppBar position="absolute" open={drawerOpen}>
                    <Toolbar
                        sx={{
                            pr: '24px', // keep right padding when drawer closed
                        }}
                        color="primary"
                    >
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
                            ntfy
                        </Typography>
                        <IconButton color="inherit">
                            <Badge badgeContent={4} color="secondary">
                                <NotificationsIcon />
                            </Badge>
                        </IconButton>
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
                        <ListItemButton onClick={() => setAddDialogOpen(true)}>
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
                    <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
                        <Grid container spacing={3}>
                            <NotificationList notifications={notifications}/>
                        </Grid>
                    </Container>
                </Box>
            </Box>
            <AddDialog
                open={addDialogOpen}
                onCancel={handleAddCancel}
                onSubmit={handleAddSubmit}
            />
        </ThemeProvider>
    );
}

export default App;
