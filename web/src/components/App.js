import * as React from 'react';
import {useEffect, useState} from 'react';
import Box from '@mui/material/Box';
import {ThemeProvider} from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import Toolbar from '@mui/material/Toolbar';
import NotificationList from "./NotificationList";
import theme from "./theme";
import api from "../app/Api";
import repository from "../app/Repository";
import connectionManager from "../app/ConnectionManager";
import Subscriptions from "../app/Subscriptions";
import Navigation from "./Navigation";
import ActionBar from "./ActionBar";
import Users from "../app/Users";

const App = () => {
    console.log(`[App] Rendering main view`);

    const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);
    const [subscriptions, setSubscriptions] = useState(new Subscriptions());
    const [users, setUsers] = useState(new Users());
    const [selectedSubscription, setSelectedSubscription] = useState(null);
    const handleNotification = (subscriptionId, notification) => {
        setSubscriptions(prev => {
            const newSubscription = prev.get(subscriptionId).addNotification(notification);
            return prev.update(newSubscription).clone();
        });
    };
    const handleSubscribeSubmit = (subscription, user) => {
        console.log(`[App] New subscription: ${subscription.id}`);
        if (user !== null) {
            setUsers(prev => prev.add(user).clone());
        }
        setSubscriptions(prev => prev.add(subscription).clone());
        setSelectedSubscription(subscription);
        api.poll(subscription.baseUrl, subscription.topic, user)
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
        setUsers(repository.loadUsers());
    }, [/* initial render only */]);
    useEffect(() => {
        connectionManager.refresh(subscriptions, users, handleNotification);
        repository.saveSubscriptions(subscriptions);
        repository.saveUsers(users);
    }, [subscriptions, users]);
    return (
        <ThemeProvider theme={theme}>
            <CssBaseline/>
            <Box sx={{display: 'flex'}}>
                <CssBaseline/>
                <ActionBar
                    selectedSubscription={selectedSubscription}
                    users={users}
                    onClearAll={handleDeleteAllNotifications}
                    onUnsubscribe={handleUnsubscribe}
                    onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)}
                />
                <Box component="nav" sx={{width: {sm: Navigation.width}, flexShrink: {sm: 0}}}>
                    <Navigation
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
                        width: {sm: `calc(100% - ${Navigation.width}px)`},
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
