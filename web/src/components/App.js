import * as React from 'react';
import {useEffect, useState} from 'react';
import Box from '@mui/material/Box';
import {ThemeProvider} from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import Toolbar from '@mui/material/Toolbar';
import Notifications from "./Notifications";
import theme from "./theme";
import connectionManager from "../app/ConnectionManager";
import Navigation from "./Navigation";
import ActionBar from "./ActionBar";
import notificationManager from "../app/NotificationManager";
import NoTopics from "./NoTopics";
import Preferences from "./Preferences";
import {useLiveQuery} from "dexie-react-hooks";
import poller from "../app/Poller";
import pruner from "../app/Pruner";
import subscriptionManager from "../app/SubscriptionManager";
import userManager from "../app/UserManager";
import {BrowserRouter, Route, Routes, useLocation, useNavigate} from "react-router-dom";
import {subscriptionRoute} from "../app/utils";

// TODO support unsubscribed routes
// TODO embed into ntfy server
// TODO googlefonts
// TODO new notification indicator
// TODO sound

const App = () => {
    return (
        <BrowserRouter>
            <ThemeProvider theme={theme}>
                <CssBaseline/>
                <Root/>
            </ThemeProvider>
        </BrowserRouter>
    );
}

const Root = () => {
    const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);
    const [notificationsGranted, setNotificationsGranted] = useState(notificationManager.granted());
    const navigate = useNavigate();
    const location = useLocation();
    const users = useLiveQuery(() => userManager.all());
    const subscriptions = useLiveQuery(() => subscriptionManager.all());
    const selectedSubscription = findSelected(location, subscriptions);

    const handleSubscriptionClick = async (subscriptionId) => {
        const subscription = await subscriptionManager.get(subscriptionId);
        navigate(subscriptionRoute(subscription));
    }
    const handleSubscribeSubmit = async (subscription) => {
        console.log(`[App] New subscription: ${subscription.id}`, subscription);
        navigate(subscriptionRoute(subscription));
        handleRequestPermission();
    };
    const handleUnsubscribe = async (subscriptionId) => {
        console.log(`[App] Unsubscribing from ${subscriptionId}`);
        const newSelected = await subscriptionManager.first(); // May be undefined
        if (newSelected) {
            navigate(subscriptionRoute(newSelected));
        }
    };
    const handleRequestPermission = () => {
        notificationManager.maybeRequestPermission(granted => setNotificationsGranted(granted));
    };
    // Define hooks: Note that the order of the hooks is important. The "loading" hooks
    // must be before the "saving" hooks.
    useEffect(() => {
        poller.startWorker();
        pruner.startWorker();
    }, [/* initial render */]);
    useEffect(() => {
        const handleNotification = async (subscriptionId, notification) => {
            try {
                const added = await subscriptionManager.addNotification(subscriptionId, notification);
                if (added) {
                    const defaultClickAction = (subscription) => navigate(subscriptionRoute(subscription)); // FIXME
                    await notificationManager.notify(subscriptionId, notification, defaultClickAction)
                }
            } catch (e) {
                console.error(`[App] Error handling notification`, e);
            }
        };
        connectionManager.registerStateListener(subscriptionManager.updateState);
        connectionManager.registerNotificationListener(handleNotification);
        return () => {
            connectionManager.resetStateListener();
            connectionManager.resetNotificationListener();
        }
    }, [/* initial render */]);
    useEffect(() => { connectionManager.refresh(subscriptions, users) }, [subscriptions, users]); // Dangle!
    return (
        <Box sx={{display: 'flex'}}>
            <CssBaseline/>
            <ActionBar
                subscriptions={subscriptions}
                selectedSubscription={selectedSubscription}
                onUnsubscribe={handleUnsubscribe}
                onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)}
            />
            <Box component="nav" sx={{width: {sm: Navigation.width}, flexShrink: {sm: 0}}}>
                <Navigation
                    subscriptions={subscriptions}
                    selectedSubscription={selectedSubscription}
                    mobileDrawerOpen={mobileDrawerOpen}
                    notificationsGranted={notificationsGranted}
                    onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)}
                    onSubscriptionClick={handleSubscriptionClick}
                    onSubscribeSubmit={handleSubscribeSubmit}
                    onRequestPermissionClick={handleRequestPermission}
                />
            </Box>
            <Main>
                <Toolbar/>
                <Routes>
                    <Route path="/" element={<NoTopics />} />
                    <Route path="settings" element={<Preferences />} />
                    <Route path=":baseUrl/:topic" element={<Notifications subscription={selectedSubscription}/>} />
                    <Route path=":topic" element={<Notifications subscription={selectedSubscription}/>} />
                </Routes>
            </Main>
        </Box>
    );
}

const Main = (props) => {
    return (
        <Box
            component="main"
            sx={{
                display: 'flex',
                flexGrow: 1,
                flexDirection: 'column',
                padding: 3,
                width: {sm: `calc(100% - ${Navigation.width}px)`},
                height: '100vh',
                overflow: 'auto',
                backgroundColor: (theme) => theme.palette.mode === 'light' ? theme.palette.grey[100] : theme.palette.grey[900]
            }}
        >
            {props.children}
        </Box>
    );
};

const findSelected = (location, subscriptions) => {
    if (!subscriptions || !location)  {
        return null;
    }
    const [subscription] = subscriptions
        .filter(s => location.pathname === subscriptionRoute(s));
    return subscription;
};

export default App;
