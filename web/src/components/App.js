import * as React from 'react';
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
import {expandUrl, topicUrl} from "../app/utils";
import ErrorBoundary from "./ErrorBoundary";
import routes from "./routes";
import {useAutoSubscribe, useBackgroundProcesses, useConnectionListeners} from "./hooks";
import Paper from "@mui/material/Paper";
import IconButton from "@mui/material/IconButton";
import TextField from "@mui/material/TextField";
import SendIcon from "@mui/icons-material/Send";
import api from "../app/Api";
import SendDialog from "./SendDialog";
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';

// TODO add drag and drop
// TODO races when two tabs are open
// TODO investigate service workers

const App = () => {
    return (
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
    );
}

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
            />
            <Main>
                <Toolbar/>
                <Outlet context={{ subscriptions, selected }}/>
            </Main>
            <Messaging selected={selected}/>
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

const Messaging = (props) => {
    const [message, setMessage] = useState("");
    const [dialogKey, setDialogKey] = useState(0);
    const [dialogOpenMode, setDialogOpenMode] = useState("");

    const subscription = props.selected;
    const selectedTopicUrl = (subscription) ? topicUrl(subscription.baseUrl, subscription.topic) : "";

    const handleOpenDialogClick = () => {
        setDialogOpenMode(SendDialog.OPEN_MODE_DEFAULT);
    };

    const handleSendDialogClose = () => {
        setDialogOpenMode("");
        setDialogKey(prev => prev+1);
    };

    return (
        <>
            {subscription && <MessageBar
                subscription={subscription}
                message={message}
                onMessageChange={setMessage}
                onOpenDialogClick={handleOpenDialogClick}
            />}
            <SendDialog
                key={`sendDialog${dialogKey}`} // Resets dialog when canceled/closed
                openMode={dialogOpenMode}
                topicUrl={selectedTopicUrl}
                message={message}
                onClose={handleSendDialogClose}
                onDragEnter={() => setDialogOpenMode(prev => (prev) ? prev : SendDialog.OPEN_MODE_DRAG)} // Only update if not already open
                onResetOpenMode={() => setDialogOpenMode(SendDialog.OPEN_MODE_DEFAULT)}
            />
        </>
    );
}

const MessageBar = (props) => {
    const subscription = props.subscription;
    const handleSendClick = () => {
        api.publish(subscription.baseUrl, subscription.topic, props.message); // FIXME
        props.onMessageChange("");
    };
    return (
        <Paper
            elevation={3}
            sx={{
                display: "flex",
                position: 'fixed',
                bottom: 0,
                right: 0,
                padding: 2,
                width: `calc(100% - ${Navigation.width}px)`,
                backgroundColor: (theme) => theme.palette.mode === 'light' ? theme.palette.grey[100] : theme.palette.grey[900]
            }}
        >
            <IconButton color="inherit" size="large" edge="start" onClick={props.onOpenDialogClick}>
                <KeyboardArrowUpIcon/>
            </IconButton>
            <TextField
                autoFocus
                margin="dense"
                placeholder="Message"
                type="text"
                fullWidth
                variant="standard"
                value={props.message}
                onChange={ev => props.onMessageChange(ev.target.value)}
                onKeyPress={(ev) => {
                    if (ev.key === 'Enter') {
                        ev.preventDefault();
                        handleSendClick();
                    }
                }}
            />
            <IconButton color="inherit" size="large" edge="end" onClick={handleSendClick}>
                <SendIcon/>
            </IconButton>
        </Paper>
    );
};

const updateTitle = (newNotificationsCount) => {
    document.title = (newNotificationsCount > 0) ? `(${newNotificationsCount}) ntfy` : "ntfy";
}

export default App;
