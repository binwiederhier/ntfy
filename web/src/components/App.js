import * as React from 'react';
import { Suspense } from "react";
import {useEffect, useState} from 'react';
import Box from '@mui/material/Box';
import {ThemeProvider} from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import Toolbar from '@mui/material/Toolbar';
import Notifications from "./Notifications";
import theme from "./theme";
import Navigation from "./Navigation";
import ActionBar from "./ActionBar";
import notifier from "../app/Notifier";
import Preferences from "./Preferences";
import {useLiveQuery} from "dexie-react-hooks";
import subscriptionManager from "../app/SubscriptionManager";
import userManager from "../app/UserManager";
import {BrowserRouter, Outlet, Route, Routes, useOutletContext, useParams} from "react-router-dom";
import {expandUrl} from "../app/utils";
import ErrorBoundary from "./ErrorBoundary";
import routes from "./routes";
import {useAutoSubscribe, useBackgroundProcesses, useConnectionListeners} from "./hooks";
import SendDialog from "./SendDialog";
import Messaging from "./Messaging";
import "./i18n"; // Translations!

// TODO races when two tabs are open
// TODO investigate service workers

const App = () => {
    return (
        <Suspense fallback={<Loader />}>
            <BrowserRouter>
                <ThemeProvider theme={theme}>
                    <CssBaseline/>
                    <ErrorBoundary>
                        <Routes>
                            <Route element={<Layout/>}>
                                <Route path={routes.root} element={<AllSubscriptions/>}/>
                                <Route path={routes.settings} element={<Preferences/>}/>
                                <Route path={routes.subscription} element={<SingleSubscription/>}/>
                                <Route path={routes.subscriptionExternal} element={<SingleSubscription/>}/>
                            </Route>
                        </Routes>
                    </ErrorBoundary>
                </ThemeProvider>
            </BrowserRouter>
        </Suspense>
    );
}

const Loader = () => (
    <div>
        <div>loading...</div>
    </div>
);

const AllSubscriptions = () => {
    const { subscriptions } = useOutletContext();
    return <Notifications mode="all" subscriptions={subscriptions}/>;
};

const SingleSubscription = () => {
    const { subscriptions, selected } = useOutletContext();
    useAutoSubscribe(subscriptions, selected);
    return <Notifications mode="one" subscription={selected}/>;
};

const Layout = () => {
    const params = useParams();
    const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);
    const [notificationsGranted, setNotificationsGranted] = useState(notifier.granted());
    const [sendDialogOpenMode, setSendDialogOpenMode] = useState("");
    const users = useLiveQuery(() => userManager.all());
    const subscriptions = useLiveQuery(() => subscriptionManager.all());
    const newNotificationsCount = subscriptions?.reduce((prev, cur) => prev + cur.new, 0) || 0;
    const [selected] = (subscriptions || []).filter(s => {
        return (params.baseUrl && expandUrl(params.baseUrl).includes(s.baseUrl) && params.topic === s.topic)
            || (window.location.origin === s.baseUrl && params.topic === s.topic)
    });

    useConnectionListeners(subscriptions, users);
    useBackgroundProcesses();
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
                onPublishMessageClick={() => setSendDialogOpenMode(SendDialog.OPEN_MODE_DEFAULT)}
            />
            <Main>
                <Toolbar/>
                <Outlet context={{ subscriptions, selected }}/>
            </Main>
            <Messaging
                selected={selected}
                dialogOpenMode={sendDialogOpenMode}
                onDialogOpenModeChange={setSendDialogOpenMode}
            />
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

const updateTitle = (newNotificationsCount) => {
    document.title = (newNotificationsCount > 0) ? `(${newNotificationsCount}) ntfy` : "ntfy";
}

export default App;
