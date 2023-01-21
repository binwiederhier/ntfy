import * as React from 'react';
import {createContext, Suspense, useContext, useEffect, useState} from 'react';
import Box from '@mui/material/Box';
import {ThemeProvider} from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import Toolbar from '@mui/material/Toolbar';
import {AllSubscriptions, SingleSubscription} from "./Notifications";
import theme from "./theme";
import Navigation from "./Navigation";
import ActionBar from "./ActionBar";
import notifier from "../app/Notifier";
import Preferences from "./Preferences";
import {useLiveQuery} from "dexie-react-hooks";
import subscriptionManager from "../app/SubscriptionManager";
import userManager from "../app/UserManager";
import {BrowserRouter, Outlet, Route, Routes, useParams} from "react-router-dom";
import {expandUrl} from "../app/utils";
import ErrorBoundary from "./ErrorBoundary";
import routes from "./routes";
import {useAccountListener, useBackgroundProcesses, useConnectionListeners} from "./hooks";
import PublishDialog from "./PublishDialog";
import Messaging from "./Messaging";
import "./i18n"; // Translations!
import {Backdrop, CircularProgress} from "@mui/material";
import Login from "./Login";
import Signup from "./Signup";
import Account from "./Account";

export const AccountContext = createContext(null);

const App = () => {
    const [account, setAccount] = useState(null);
    return (
        <Suspense fallback={<Loader />}>
            <BrowserRouter>
                <ThemeProvider theme={theme}>
                    <AccountContext.Provider value={{ account, setAccount }}>
                        <CssBaseline/>
                        <ErrorBoundary>
                            <Routes>
                                <Route path={routes.login} element={<Login/>}/>
                                <Route path={routes.signup} element={<Signup/>}/>
                                <Route element={<Layout/>}>
                                    <Route path={routes.app} element={<AllSubscriptions/>}/>
                                    <Route path={routes.account} element={<Account/>}/>
                                    <Route path={routes.settings} element={<Preferences/>}/>
                                    <Route path={routes.subscription} element={<SingleSubscription/>}/>
                                    <Route path={routes.subscriptionExternal} element={<SingleSubscription/>}/>
                                </Route>
                            </Routes>
                        </ErrorBoundary>
                    </AccountContext.Provider>
                </ThemeProvider>
            </BrowserRouter>
        </Suspense>
    );
}

const Layout = () => {
    const params = useParams();
    const { account, setAccount } = useContext(AccountContext);
    const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);
    const [notificationsGranted, setNotificationsGranted] = useState(notifier.granted());
    const [sendDialogOpenMode, setSendDialogOpenMode] = useState("");
    const users = useLiveQuery(() => userManager.all());
    const subscriptions = useLiveQuery(() => subscriptionManager.all());
    const subscriptionsWithoutInternal = subscriptions?.filter(s => !s.internal);
    const newNotificationsCount = subscriptionsWithoutInternal?.reduce((prev, cur) => prev + cur.new, 0) || 0;
    const [selected] = (subscriptionsWithoutInternal || []).filter(s => {
        return (params.baseUrl && expandUrl(params.baseUrl).includes(s.baseUrl) && params.topic === s.topic)
            || (config.base_url === s.baseUrl && params.topic === s.topic)
    });

    useConnectionListeners(subscriptions, users);
    useAccountListener(setAccount)
    useBackgroundProcesses();
    useEffect(() => updateTitle(newNotificationsCount), [newNotificationsCount]);

    useEffect(() => {
        if (!account || !account.sync_topic) {
            return;
        }
        (async () => {
            const subscription = await subscriptionManager.add(config.base_url, account.sync_topic);
            if (!subscription.hidden) {
                await subscriptionManager.update(subscription.id, {
                    internal: true
                });
            }
        })();
    }, [account]);

    return (
        <Box sx={{display: 'flex'}}>
            <ActionBar
                selected={selected}
                onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)}
            />
            <Navigation
                subscriptions={subscriptionsWithoutInternal}
                selectedSubscription={selected}
                notificationsGranted={notificationsGranted}
                mobileDrawerOpen={mobileDrawerOpen}
                onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)}
                onNotificationGranted={setNotificationsGranted}
                onPublishMessageClick={() => setSendDialogOpenMode(PublishDialog.OPEN_MODE_DEFAULT)}
            />
            <Main>
                <Toolbar/>
                <Outlet context={{
                    subscriptions: subscriptionsWithoutInternal,
                    selected: selected
                }}/>
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

const Loader = () => (
    <Backdrop
        open={true}
        sx={{
            zIndex: 100000,
            backgroundColor: (theme) => theme.palette.mode === 'light' ? theme.palette.grey[100] : theme.palette.grey[900]
        }}
    >
        <CircularProgress color="success" disableShrink />
    </Backdrop>
);

const updateTitle = (newNotificationsCount) => {
    document.title = (newNotificationsCount > 0) ? `(${newNotificationsCount}) ntfy` : "ntfy";
}

export default App;
