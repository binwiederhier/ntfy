import * as React from "react";
import { createContext, Suspense, useContext, useEffect, useState, useMemo } from "react";
import { Box, Toolbar, CssBaseline, Backdrop, CircularProgress, useMediaQuery, ThemeProvider, createTheme } from "@mui/material";
import { useLiveQuery } from "dexie-react-hooks";
import { BrowserRouter, Outlet, Route, Routes, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { AllSubscriptions, SingleSubscription } from "./Notifications";
import { darkTheme, lightTheme } from "./theme";
import Navigation from "./Navigation";
import ActionBar from "./ActionBar";
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
import initI18n from "../app/i18n"; // Translations!
import prefs, { THEME } from "../app/Prefs";
import RTLCacheProvider from "./RTLCacheProvider";

initI18n();

export const AccountContext = createContext(null);

const darkModeEnabled = (prefersDarkMode, themePreference) => {
  switch (themePreference) {
    case THEME.DARK:
      return true;

    case THEME.LIGHT:
      return false;

    case THEME.SYSTEM:
    default:
      return prefersDarkMode;
  }
};

const App = () => {
  const { i18n } = useTranslation();
  const languageDir = i18n.dir();

  const [account, setAccount] = useState(null);
  const accountMemo = useMemo(() => ({ account, setAccount }), [account, setAccount]);
  const prefersDarkMode = useMediaQuery("(prefers-color-scheme: dark)");
  const themePreference = useLiveQuery(() => prefs.theme());
  const theme = React.useMemo(
    () => createTheme({ ...(darkModeEnabled(prefersDarkMode, themePreference) ? darkTheme : lightTheme), direction: languageDir }),
    [prefersDarkMode, themePreference, languageDir]
  );

  useEffect(() => {
    document.documentElement.setAttribute("lang", i18n.language);
    document.dir = languageDir;
  }, [i18n.language, languageDir]);

  return (
    <Suspense fallback={<Loader />}>
      <RTLCacheProvider>
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
      </RTLCacheProvider>
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
        mobileDrawerOpen={mobileDrawerOpen}
        onMobileDrawerToggle={() => setMobileDrawerOpen(!mobileDrawerOpen)}
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
      padding: { xs: 0, md: 3 },
      width: { sm: `calc(100% - ${Navigation.width}px)` },
      height: "100dvh",
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
