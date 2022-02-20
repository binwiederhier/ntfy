import * as React from 'react';
import {useState} from 'react';
import Container from '@mui/material/Container';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import WsConnection from './WsConnection';
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
import {Button, CardActions, CardContent, Stack} from "@mui/material";
import AddDialog from "./AddDialog";
import theme from "./theme";

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
    const [drawerOpen, setDrawerOpen] = useState(true);
    const [subscriptions, setSubscriptions] = useState({});
    const [selectedSubscription, setSelectedSubscription] = useState(null);
    const [connections, setConnections] = useState({});
    const [addDialogOpen, setAddDialogOpen] = useState(false);
    const subscriptionChanged = (subscription) => {
        setSubscriptions(prev => ({...prev, [subscription.id]: subscription})); // Fake-replace
    };
    const handleAddSubmit = (subscription) => {
        setAddDialogOpen(false);
        const connection = new WsConnection(subscription, subscriptionChanged);
        setSubscriptions(prev => ({...prev, [subscription.id]: subscription}));
        setConnections(prev => ({...prev, [connection.id]: connection}));
        connection.start();
    };
    const handleAddCancel = () => {
        setAddDialogOpen(false);
    }
    const handleSubscriptionClick = (subscriptionId) => {
        console.log(`handleSubscriptionClick ${subscriptionId}`)
        setSelectedSubscription(subscriptions[subscriptionId]);
    };
    const notifications = (selectedSubscription !== null) ? selectedSubscription.notifications : [];
    const toggleDrawer = () => {
        setDrawerOpen(!drawerOpen);
    };
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
