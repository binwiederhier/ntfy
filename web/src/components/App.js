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
import {BrowserRouter, Route, Routes, useNavigate, useParams} from "react-router-dom";
import {expandUrl, subscriptionRoute} from "../app/utils";

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
                <Content/>
            </ThemeProvider>
        </BrowserRouter>
    );
}

const Content = () => {
    const subscriptions = useLiveQuery(() => subscriptionManager.all());
    // const context = { subscriptions };
    return (
        <Routes>
            <Route path="settings" element={<PrefLayout subscriptions={subscriptions}/>} />
            <Route path="settings" element={<PrefLayout subscriptions={subscriptions}/>} />
            <Route path="/" element={<AllSubscriptions subscriptions={subscriptions}/>} />
            <Route path=":baseUrl/:topic" element={<SingleSubscription subscriptions={subscriptions}/>} />
            <Route path=":topic" element={<SingleSubscription subscriptions={subscriptions}/>} />
        </Routes>
    )
};

const AllSubscriptions = (props) => {
    return (
        <Layout subscriptions={props.subscriptions}>
            <Notifications mode="all" subscriptions={props.subscriptions}/>
        </Layout>
    );
}

const SingleSubscription = (props) => {
    const params = useParams();
    const [selected] = (props.subscriptions || []).filter(s => {
        return (params.baseUrl && expandUrl(params.baseUrl).includes(s.baseUrl) && params.topic === s.topic)
            || (window.location.origin === s.baseUrl && params.topic === s.topic)
    });
    return (
        <Layout subscriptions={props.subscriptions} selected={selected}>
            <Notifications mode="one" subscription={selected}/>
        </Layout>
    );
}

const PrefLayout = (props) => {
    return (
        <Layout subscriptions={props.subscriptions}>
            <Preferences/>
        </Layout>
    );
}

const Layout = (props) => {
    const subscriptions = props.subscriptions; // May be null/undefined
    const selected = props.selected; // May be null/undefined
    const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);
    const [notificationsGranted, setNotificationsGranted] = useState(notifier.granted());
    const users = useLiveQuery(() => userManager.all());
    const newNotificationsCount = subscriptions?.reduce((prev, cur) => prev + cur.new, 0) || 0;

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
                {props.children}
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
