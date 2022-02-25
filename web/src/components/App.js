import * as React from 'react';
import {useEffect, useState} from 'react';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import {ThemeProvider} from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import Drawer from '@mui/material/Drawer';
import AppBar from '@mui/material/AppBar';
import Toolbar from '@mui/material/Toolbar';
import ChatBubbleOutlineIcon from '@mui/icons-material/ChatBubbleOutline';
import List from '@mui/material/List';
import Divider from '@mui/material/Divider';
import IconButton from '@mui/material/IconButton';
import MenuIcon from '@mui/icons-material/Menu';
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText from "@mui/material/ListItemText";
import ListItemButton from "@mui/material/ListItemButton";
import SettingsIcon from "@mui/icons-material/Settings";
import AddIcon from "@mui/icons-material/Add";
import SubscribeDialog from "./SubscribeDialog";
import NotificationList from "./NotificationList";
import IconSubscribeSettings from "./IconSubscribeSettings";
import theme from "./theme";
import api from "../app/Api";
import repository from "../app/Repository";
import connectionManager from "../app/ConnectionManager";
import Subscriptions from "../app/Subscriptions";

const drawerWidth = 240;

const NavSubscriptionList = (props) => {
    const subscriptions = props.subscriptions;
    return (
        <>
            {subscriptions.map((id, subscription) =>
                <NavSubscriptionItem
                    key={id}
                    subscription={subscription}
                    selected={props.selectedSubscription && props.selectedSubscription.id === id}
                    onClick={() => props.onSubscriptionClick(id)}
                />)
            }
        </>
    );
}

const NavSubscriptionItem = (props) => {
    const subscription = props.subscription;
    return (
        <ListItemButton onClick={props.onClick} selected={props.selected}>
            <ListItemIcon><ChatBubbleOutlineIcon /></ListItemIcon>
            <ListItemText primary={subscription.shortUrl()}/>
        </ListItemButton>
    );
}

const NavList = (props) => {
    const [subscribeDialogOpen, setSubscribeDialogOpen] = useState(false);
    const handleSubscribeSubmit = (subscription) => {
        setSubscribeDialogOpen(false);
        props.onSubscribeSubmit(subscription);
    }
    return (
        <>
            <Toolbar />
            <Divider />
            <List component="nav">
                <NavSubscriptionList
                    subscriptions={props.subscriptions}
                    selectedSubscription={props.selectedSubscription}
                    onSubscriptionClick={props.onSubscriptionClick}
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
            <SubscribeDialog
                open={subscribeDialogOpen}
                onCancel={() => setSubscribeDialogOpen(false)}
                onSubmit={handleSubscribeSubmit}
            />
        </>
    );
};

const ActionBar = (props) => {
    const title = (props.selectedSubscription !== null)
        ? props.selectedSubscription.shortUrl()
        : "ntfy";
    return (
        <AppBar position="fixed" sx={{ width: { sm: `calc(100% - ${drawerWidth}px)` }, ml: { sm: `${drawerWidth}px` } }}>
            <Toolbar sx={{pr: '24px'}}>
                <IconButton
                    color="inherit"
                    edge="start"
                    onClick={props.onMobileDrawerToggle}
                    sx={{ mr: 2, display: { sm: 'none' } }}
                >
                    <MenuIcon />
                </IconButton>
                <Typography variant="h6" noWrap component="div" sx={{ flexGrow: 1 }}>{title}</Typography>
                {props.selectedSubscription !== null && <IconSubscribeSettings
                    subscription={props.selectedSubscription}
                    onClearAll={props.onClearAll}
                    onUnsubscribe={props.onUnsubscribe}
                />}
            </Toolbar>
        </AppBar>
    );
};

const Sidebar = (props) => {
    const navigationList =
        <NavList
            subscriptions={props.subscriptions}
            selectedSubscription={props.selectedSubscription}
            onSubscriptionClick={props.onSubscriptionClick}
            onSubscribeSubmit={props.onSubscribeSubmit}
        />;
    return (
        <>
            {/* Mobile drawer; only shown if menu icon clicked (mobile open) and display is small */}
            <Drawer
                variant="temporary"
                open={props.mobileDrawerOpen}
                onClose={props.onMobileDrawerToggle}
                ModalProps={{ keepMounted: true }} // Better open performance on mobile.
                sx={{
                    display: { xs: 'block', sm: 'none' },
                    '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth },
                }}
            >
                {navigationList}
            </Drawer>
            {/* Big screen drawer; persistent, shown if screen is big */}
            <Drawer
                open
                variant="permanent"
                sx={{
                    display: { xs: 'none', sm: 'block' },
                    '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth },
                }}
            >
                {navigationList}
            </Drawer>
        </>
    );
};

const App = () => {
    console.log(`[App] Rendering main view`);

    const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);
    const [subscriptions, setSubscriptions] = useState(new Subscriptions());
    const [selectedSubscription, setSelectedSubscription] = useState(null);
    const handleNotification = (subscriptionId, notification) => {
        setSubscriptions(prev => {
            const newSubscription = prev.get(subscriptionId).addNotification(notification);
            return prev.update(newSubscription).clone();
        });
    };
    const handleSubscribeSubmit = (subscription) => {
        console.log(`[App] New subscription: ${subscription.id}`);
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
    const handleDeleteNotification = (subscriptionId, notificationId) => {
        console.log(`[App] Deleting notification ${notificationId} from ${subscriptionId}`);
        setSubscriptions(prev => {
            const newSubscription = prev.get(subscriptionId).deleteNotification(notificationId);
            return prev.update(newSubscription).clone();
        });
    };
    const handleDeleteAllNotifications = (subscriptionId) => {
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
    useEffect(() => {
        setSubscriptions(repository.loadSubscriptions());
    }, [/* initial render only */]);
    useEffect(() => {
        connectionManager.refresh(subscriptions, handleNotification);
        repository.saveSubscriptions(subscriptions);
    }, [subscriptions]);

    return (
        <ThemeProvider theme={theme}>
            <CssBaseline/>
            <Box sx={{display: 'flex'}}>
                <CssBaseline/>
                <ActionBar
                    selectedSubscription={selectedSubscription}
                    onClearAll={handleDeleteAllNotifications}
                    onUnsubscribe={handleUnsubscribe}
                    onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)}
                />
                <Box component="nav" sx={{width: {sm: drawerWidth}, flexShrink: {sm: 0}}}>
                    <Sidebar
                        subscriptions={subscriptions}
                        selectedSubscription={selectedSubscription}
                        mobileDrawerOpen={mobileDrawerOpen}
                        onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)}
                        onSubscriptionClick={(subscriptionId) => setSelectedSubscription(subscriptions.get(subscriptionId))}
                        onSubscribeSubmit={handleSubscribeSubmit}
                    />
                </Box>
                <Box
                    component="main"
                    sx={{
                        flexGrow: 1,
                        p: 3,
                        width: {sm: `calc(100% - ${drawerWidth}px)`},
                        height: '100vh',
                        overflow: 'auto',
                        backgroundColor: (theme) => theme.palette.mode === 'light' ? theme.palette.grey[100] : theme.palette.grey[900]
                }}>
                    <Toolbar/>
                    {selectedSubscription !== null &&
                        <NotificationList
                            subscription={selectedSubscription}
                            onDeleteNotification={handleDeleteNotification}
                        />}
                </Box>
            </Box>
        </ThemeProvider>
    );
}

export default App;
