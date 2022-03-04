import * as React from 'react';
import {useEffect, useState} from 'react';
import Box from '@mui/material/Box';
import {ThemeProvider} from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import Toolbar from '@mui/material/Toolbar';
import Notifications from "./Notifications";
import theme from "./theme";
import prefs from "../app/Prefs";
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

// TODO make default server functional
// TODO routing
// TODO embed into ntfy server
// TODO new notification indicator

const App = () => {
    const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);
    const [prefsOpen, setPrefsOpen] = useState(false);
    const [selectedSubscription, setSelectedSubscription] = useState(null);
    const [notificationsGranted, setNotificationsGranted] = useState(notificationManager.granted());
    const subscriptions = useLiveQuery(() => subscriptionManager.all());
    const users = useLiveQuery(() => userManager.all());
    const handleSubscriptionClick = async (subscriptionId) => {
        const subscription = await subscriptionManager.get(subscriptionId);
        setSelectedSubscription(subscription);
        setPrefsOpen(false);
    }
    const handleSubscribeSubmit = async (subscription) => {
        console.log(`[App] New subscription: ${subscription.id}`, subscription);
        setSelectedSubscription(subscription);
        handleRequestPermission();
    };
    const handleUnsubscribe = async (subscriptionId) => {
        console.log(`[App] Unsubscribing from ${subscriptionId}`);
        const newSelected = await subscriptionManager.first(); // May be undefined
        setSelectedSubscription(newSelected);
    };
    const handleRequestPermission = () => {
        notificationManager.maybeRequestPermission(granted => setNotificationsGranted(granted));
    };
    const handlePrefsClick = () => {
        setPrefsOpen(true);
        setSelectedSubscription(null);
    };
    // Define hooks: Note that the order of the hooks is important. The "loading" hooks
    // must be before the "saving" hooks.
    useEffect(() => {
        poller.startWorker();
        pruner.startWorker();
        const load = async () => {
            const subs = await subscriptionManager.all();             // FIXME this is broken
            const selectedSubscriptionId = await prefs.selectedSubscriptionId();

            // Set selected subscription
            const maybeSelectedSubscription = subs?.filter(s => s.id = selectedSubscriptionId);
            if (maybeSelectedSubscription.length > 0) {
                setSelectedSubscription(maybeSelectedSubscription[0]);
            }

        };
        setTimeout(() => load(), 5000);
    }, [/* initial render */]);
    useEffect(() => {
        const handleNotification = async (subscriptionId, notification) => {
            try {
                const added = await subscriptionManager.addNotification(subscriptionId, notification);
                if (added) {
                    const defaultClickAction = (subscription) => setSelectedSubscription(subscription);
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
    useEffect(() => {
        connectionManager.refresh(subscriptions, users); // Dangle
    }, [subscriptions, users]);
    useEffect(() => {
        const subscriptionId = (selectedSubscription) ? selectedSubscription.id : "";
        prefs.setSelectedSubscriptionId(subscriptionId)
    }, [selectedSubscription]);

    return (
        <ThemeProvider theme={theme}>
            <CssBaseline/>
            <Box sx={{display: 'flex'}}>
                <CssBaseline/>
                <ActionBar
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
                        prefsOpen={prefsOpen}
                        onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)}
                        onSubscriptionClick={handleSubscriptionClick}
                        onSubscribeSubmit={handleSubscribeSubmit}
                        onPrefsClick={handlePrefsClick}
                        onRequestPermissionClick={handleRequestPermission}
                    />
                </Box>
                <Main>
                    <Toolbar/>
                    <Content
                        subscription={selectedSubscription}
                        prefsOpen={prefsOpen}
                    />
                </Main>
            </Box>
        </ThemeProvider>
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

const Content = (props) => {
    if (props.prefsOpen) {
        return <Preferences/>;
    }
    if (props.subscription) {
        return <Notifications subscription={props.subscription}/>;
    }
    return <NoTopics/>;
};

export default App;
