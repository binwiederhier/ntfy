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
import notifier from "../app/Notifier";
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
// TODO add "home" route that is selected when nothing else fits
// TODO new notification indicator
// TODO "copy url" toast
// TODO "copy link url" button
// TODO races when two tabs are open
// TODO investigate service workers

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
    const [notificationsGranted, setNotificationsGranted] = useState(notifier.granted());
    const location = useLocation();
    const users = useLiveQuery(() => userManager.all());
    const subscriptions = useLiveQuery(() => subscriptionManager.all());
    const selectedSubscription = findSelected(location, subscriptions);
    const newNotificationsCount = subscriptions?.reduce((prev, cur) => prev + cur.new, 0) || 0;

    useWorkers();
    useConnectionListeners();

    useEffect(() => {
        connectionManager.refresh(subscriptions, users);
    }, [subscriptions, users]); // Dangle!

    useEffect(() => {
        console.log(`hello ${newNotificationsCount}`)
        document.title = (newNotificationsCount > 0) ? `(${newNotificationsCount}) ntfy web` : "ntfy web";
    }, [newNotificationsCount]);

    return (
        <Box sx={{display: 'flex'}}>
            <CssBaseline/>
            <ActionBar
                subscriptions={subscriptions}
                selectedSubscription={selectedSubscription}
                onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)}
            />
            <Box component="nav" sx={{width: {sm: Navigation.width}, flexShrink: {sm: 0}}}>
                <Navigation
                    subscriptions={subscriptions}
                    selectedSubscription={selectedSubscription}
                    notificationsGranted={notificationsGranted}
                    requestNotificationPermission={() => notifier.maybeRequestPermission(granted => setNotificationsGranted(granted))}
                    mobileDrawerOpen={mobileDrawerOpen}
                    onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)}
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
    const [subscription] = subscriptions.filter(s => location.pathname === subscriptionRoute(s));
    return subscription;

    /*
    if (location.pathname === "/" || location.pathname === "/settings") {
        return null;
    }
    if (!subscription) {
        const [, topic] = location.pathname.split("/");
        const subscription = {
            id: topicUrl(window.location.origin, topic),
            baseUrl: window.location.origin,
            topic: topic,
            last: ""
        }
        subscriptionManager.save(subscription);
        return subscription;
    }

     */
};


const useWorkers = () => {
    useEffect(() => {
        poller.startWorker();
        pruner.startWorker();
    }, []);
};

const useConnectionListeners = () => {
    const navigate = useNavigate();
    useEffect(() => {
        const handleNotification = async (subscriptionId, notification) => {
            const added = await subscriptionManager.addNotification(subscriptionId, notification);
            if (added) {
                const defaultClickAction = (subscription) => navigate(subscriptionRoute(subscription));
                await notifier.notify(subscriptionId, notification, defaultClickAction)
            }
        };
        connectionManager.registerStateListener(subscriptionManager.updateState);
        connectionManager.registerNotificationListener(handleNotification);
        return () => {
            connectionManager.resetStateListener();
            connectionManager.resetNotificationListener();
        }
    },
    // We have to disable dep checking for "navigate". This is fine, it never changes.
    // eslint-disable-next-line
    []);
};

export default App;
