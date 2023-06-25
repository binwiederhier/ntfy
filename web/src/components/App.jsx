import * as React from "react";
import { createContext, Suspense, useContext, useEffect, useState, useMemo } from "react";
import { Box, Toolbar, CssBaseline, Backdrop, CircularProgress } from "@mui/material";
import { ThemeProvider } from "@mui/material/styles";
import { useLiveQuery } from "dexie-react-hooks";
import { BrowserRouter, Outlet, Route, Routes, useParams } from "react-router-dom";
import { AllSubscriptions, SingleSubscription } from "./Notifications";
import theme from "./theme";
import Navigation from "./Navigation";
import ActionBar from "./ActionBar";
import notifier from "../app/Notifier";
import Preferences from "./Preferences";
import subscriptionManager from "../app/SubscriptionManager";
import userManager from "../app/UserManager";
import { expandUrl } from "../app/utils";
import ErrorBoundary from "./ErrorBoundary";
import routes from "./routes";
import { useAccountListener, useBackgroundProcesses, useConnectionListeners, useWebPushTopics } from "./hooks";
import PublishDialog from "./PublishDialog";
import Messaging from "./Messaging";
import Login from "./Login";
import Signup from "./Signup";
import Account from "./Account";
import "../app/i18n"; // Translations!

export const AccountContext = createContext(null);

const App = () => {
  const [account, setAccount] = useState(null);
  const accountMemo = useMemo(() => ({ account, setAccount }), [account, setAccount]);

  return (
    <Suspense fallback={<Loader />}>
      <BrowserRouter>
        <ThemeProvider theme={theme}>
          <AccountContext.Provider value={accountMemo}>
            <CssBaseline />
            <ErrorBoundary>
              <Routes>
                <Route path={routes.login} element={<Login />} />
                <Route path={routes.signup} element={<Signup />} />
                <Route element={<Layout />}>
                  <Route path={routes.app} element={<AllSubscriptions />} />
                  <Route path={routes.account} element={<Account />} />
                  <Route path={routes.settings} element={<Preferences />} />
                  <Route path={routes.subscription} element={<SingleSubscription />} />
                  <Route path={routes.subscriptionExternal} element={<SingleSubscription />} />
                </Route>
              </Routes>
            </ErrorBoundary>
          </AccountContext.Provider>
        </ThemeProvider>
      </BrowserRouter>
    </Suspense>
  );
};

const updateTitle = (newNotificationsCount) => {
  document.title = newNotificationsCount > 0 ? `(${newNotificationsCount}) ntfy` : "ntfy";
  window.navigator.setAppBadge?.(newNotificationsCount);
};

const Layout = () => {
  const params = useParams();
  const { account, setAccount } = useContext(AccountContext);
  const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);
  const [notificationsGranted, setNotificationsGranted] = useState(notifier.granted());
  const [sendDialogOpenMode, setSendDialogOpenMode] = useState("");
  const users = useLiveQuery(() => userManager.all());
  const subscriptions = useLiveQuery(() => subscriptionManager.all());
  const webPushTopics = useWebPushTopics();
  const subscriptionsWithoutInternal = subscriptions?.filter((s) => !s.internal);
  const newNotificationsCount = subscriptionsWithoutInternal?.reduce((prev, cur) => prev + cur.new, 0) || 0;
  const [selected] = (subscriptionsWithoutInternal || []).filter(
    (s) =>
      (params.baseUrl && expandUrl(params.baseUrl).includes(s.baseUrl) && params.topic === s.topic) ||
      (config.base_url === s.baseUrl && params.topic === s.topic)
  );

  useConnectionListeners(account, subscriptions, users, webPushTopics);
  useAccountListener(setAccount);
  useBackgroundProcesses();
  useEffect(() => updateTitle(newNotificationsCount), [newNotificationsCount]);

  return (
    <Box sx={{ display: "flex" }}>
      <ActionBar selected={selected} onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)} />
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
        <Toolbar />
        <Outlet
          context={{
            subscriptions: subscriptionsWithoutInternal,
            selected,
          }}
        />
      </Main>
      <Messaging selected={selected} dialogOpenMode={sendDialogOpenMode} onDialogOpenModeChange={setSendDialogOpenMode} />
    </Box>
  );
};

const Main = (props) => (
  <Box
    id="main"
    component="main"
    sx={{
      display: "flex",
      flexGrow: 1,
      flexDirection: "column",
      padding: 3,
      width: { sm: `calc(100% - ${Navigation.width}px)` },
      height: "100vh",
      overflow: "auto",
      backgroundColor: ({ palette }) => (palette.mode === "light" ? palette.grey[100] : palette.grey[900]),
    }}
  >
    {props.children}
  </Box>
);

const Loader = () => (
  <Backdrop
    open
    sx={{
      zIndex: 100000,
      backgroundColor: ({ palette }) => (palette.mode === "light" ? palette.grey[100] : palette.grey[900]),
    }}
  >
    <CircularProgress color="success" disableShrink />
  </Backdrop>
);

export default App;
