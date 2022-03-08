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
import Preferences from "./Preferences";
import {useLiveQuery} from "dexie-react-hooks";
import subscriptionManager from "../app/SubscriptionManager";
import userManager from "../app/UserManager";
import {BrowserRouter, Outlet, Route, Routes, useNavigate, useOutletContext, useParams} from "react-router-dom";
import {expandSecureUrl, expandUrl, subscriptionRoute, topicUrl} from "../app/utils";

// TODO support unsubscribed routes
// TODO "copy url" toast
// TODO "copy link url" button
// TODO races when two tabs are open
// TODO investigate service workers

const App = () => {
    return (
        <BrowserRouter>
            <ThemeProvider theme={theme}>
                <CssBaseline/>
                <Routes>
                    <Route element={<Layout/>}>
                        <Route path="/" element={<AllSubscriptions/>} />
                        <Route path="settings" element={<Preferences/>} />
                        <Route path=":topic" element={<SingleSubscription/>} />
                        <Route path=":baseUrl/:topic" element={<SingleSubscription/>} />
                    </Route>
                </Routes>
            </ThemeProvider>
        </BrowserRouter>
    );
}

const AllSubscriptions = () => {
    const { subscriptions } = useOutletContext();
    return <Notifications mode="all" subscriptions={subscriptions}/>;
};

const SingleSubscription = () => {
    const { subscriptions, selected } = useOutletContext();
    const [missingAdded, setMissingAdded] = useState(false);
    const params = useParams();
    useEffect(() => {
        const loaded = subscriptions !== null && subscriptions !== undefined;
        const missing = loaded && params.topic && !selected && !missingAdded;
        if (missing) {
            setMissingAdded(true);
            const baseUrl = (params.baseUrl) ? expandSecureUrl(params.baseUrl) : window.location.origin;
            console.log(`[App] Adding ephemeral subscription for ${topicUrl(baseUrl, params.topic)}`);
            // subscriptionManager.add(baseUrl, params.topic, true); // Dangle!
        }
    }, [params, subscriptions, selected, missingAdded]);

    return <Notifications mode="one" subscription={selected}/>;
};

const Layout = () => {
    const params = useParams();
    const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);
    const [notificationsGranted, setNotificationsGranted] = useState(notifier.granted());
    const users = useLiveQuery(() => userManager.all());
    const subscriptions = useLiveQuery(() => subscriptionManager.all());
    const newNotificationsCount = subscriptions?.reduce((prev, cur) => prev + cur.new, 0) || 0;
    const [selected] = (subscriptions || []).filter(s => {
        return (params.baseUrl && expandUrl(params.baseUrl).includes(s.baseUrl) && params.topic === s.topic)
            || (window.location.origin === s.baseUrl && params.topic === s.topic)
    });

    useConnectionListeners();

    useEffect(() => connectionManager.refresh(subscriptions, users), [subscriptions, users]);
    useEffect(() => updateTitle(newNotificationsCount), [newNotificationsCount]);

    return (
        <Box sx={{display: 'flex'}}>
            <CssBaseline/>
            <ActionBar
                selected={selected}
                onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)}
            />
            <Navigation
                subscriptions={subscriptions}
                selectedSubscription={selected}
                notificationsGranted={notificationsGranted}
                mobileDrawerOpen={mobileDrawerOpen}
                onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)}
                onNotificationGranted={setNotificationsGranted}
            />
            <Main>
                <Toolbar/>
                <Outlet context={{ subscriptions, selected }}/>
            </Main>
        </Box>
    );
}

const Main = (props) => {
    return (
        <Box
            id="main"
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

const updateTitle = (newNotificationsCount) => {
    document.title = (newNotificationsCount > 0) ? `(${newNotificationsCount}) ntfy web` : "ntfy web";
}

export default App;
