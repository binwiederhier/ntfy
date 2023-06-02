import * as React from "react";
import { useContext, useState } from "react";
import {
  Button,
  TextField,
  Dialog,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Autocomplete,
  FormControlLabel,
  FormGroup,
  useMediaQuery,
  Switch,
} from "@mui/material";
import { useTranslation } from "react-i18next";
import { useLiveQuery } from "dexie-react-hooks";
import theme from "./theme";
import api from "../app/Api";
import { randomAlphanumericString, topicUrl, validTopic, validUrl } from "../app/utils";
import userManager from "../app/UserManager";
import subscriptionManager from "../app/SubscriptionManager";
import poller from "../app/Poller";
import DialogFooter from "./DialogFooter";
import session from "../app/Session";
import routes from "./routes";
import accountApi, { Permission, Role } from "../app/AccountApi";
import ReserveTopicSelect from "./ReserveTopicSelect";
import { AccountContext } from "./App";
import { TopicReservedError, UnauthorizedError } from "../app/errors";
import { ReserveLimitChip } from "./SubscriptionPopup";
import notifier from "../app/Notifier";
import prefs from "../app/Prefs";

const publicBaseUrl = "https://ntfy.sh";

export const subscribeTopic = async (baseUrl, topic, opts) => {
  const subscription = await subscriptionManager.add(baseUrl, topic, opts);
  if (session.exists()) {
    try {
      await accountApi.addSubscription(baseUrl, topic);
    } catch (e) {
      console.log(`[SubscribeDialog] Subscribing to topic ${topic} failed`, e);
      if (e instanceof UnauthorizedError) {
        session.resetAndRedirect(routes.login);
      }
    }
  }
  return subscription;
};

const SubscribeDialog = (props) => {
  const [baseUrl, setBaseUrl] = useState("");
  const [topic, setTopic] = useState("");
  const [showLoginPage, setShowLoginPage] = useState(false);
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));

  const webPushDefaultEnabled = useLiveQuery(async () => prefs.webPushDefaultEnabled());

  const handleSuccess = async (webPushEnabled) => {
    console.log(`[SubscribeDialog] Subscribing to topic ${topic}`);
    const actualBaseUrl = baseUrl || config.base_url;
    const subscription = await subscribeTopic(actualBaseUrl, topic, {
      webPushEnabled,
    });
    poller.pollInBackground(subscription); // Dangle!

    // if the user hasn't changed the default web push setting yet, set it to enabled
    if (webPushEnabled && webPushDefaultEnabled === "initial") {
      await prefs.setWebPushDefaultEnabled(true);
    }

    props.onSuccess(subscription);
  };

  // wait for liveQuery load
  if (webPushDefaultEnabled === undefined) {
    return <></>;
  }

  return (
    <Dialog open={props.open} onClose={props.onCancel} fullScreen={fullScreen}>
      {!showLoginPage && (
        <SubscribePage
          baseUrl={baseUrl}
          setBaseUrl={setBaseUrl}
          topic={topic}
          setTopic={setTopic}
          subscriptions={props.subscriptions}
          onCancel={props.onCancel}
          onNeedsLogin={() => setShowLoginPage(true)}
          onSuccess={handleSuccess}
          webPushDefaultEnabled={webPushDefaultEnabled}
        />
      )}
      {showLoginPage && <LoginPage baseUrl={baseUrl} topic={topic} onBack={() => setShowLoginPage(false)} onSuccess={handleSuccess} />}
    </Dialog>
  );
};

const SubscribePage = (props) => {
  const { t } = useTranslation();
  const { account } = useContext(AccountContext);
  const [error, setError] = useState("");
  const [reserveTopicVisible, setReserveTopicVisible] = useState(false);
  const [anotherServerVisible, setAnotherServerVisible] = useState(false);
  const [everyone, setEveryone] = useState(Permission.DENY_ALL);
  const baseUrl = anotherServerVisible ? props.baseUrl : config.base_url;
  const { topic } = props;
  const existingTopicUrls = props.subscriptions.map((s) => topicUrl(s.baseUrl, s.topic));
  const existingBaseUrls = Array.from(new Set([publicBaseUrl, ...props.subscriptions.map((s) => s.baseUrl)])).filter(
    (s) => s !== config.base_url
  );
  const showReserveTopicCheckbox = config.enable_reservations && !anotherServerVisible && (config.enable_payments || account);
  const reserveTopicEnabled =
    session.exists() && (account?.role === Role.ADMIN || (account?.role === Role.USER && (account?.stats.reservations_remaining || 0) > 0));

  const [backgroundNotificationsEnabled, setBackgroundNotificationsEnabled] = useState(props.webPushDefaultEnabled === "enabled");

  const handleBackgroundNotificationsChanged = (e) => {
    setBackgroundNotificationsEnabled(e.target.checked);
  };

  const handleSubscribe = async () => {
    const user = await userManager.get(baseUrl); // May be undefined
    const username = user ? user.username : t("subscribe_dialog_error_user_anonymous");

    // Check read access to topic
    const success = await api.topicAuth(baseUrl, topic, user);
    if (!success) {
      console.log(`[SubscribeDialog] Login to ${topicUrl(baseUrl, topic)} failed for user ${username}`);
      if (user) {
        setError(
          t("subscribe_dialog_error_user_not_authorized", {
            username,
          })
        );
        return;
      }
      props.onNeedsLogin();
      return;
    }

    // Reserve topic (if requested)
    if (session.exists() && baseUrl === config.base_url && reserveTopicVisible) {
      console.log(`[SubscribeDialog] Reserving topic ${topic} with everyone access ${everyone}`);
      try {
        await accountApi.upsertReservation(topic, everyone);
      } catch (e) {
        console.log(`[SubscribeDialog] Error reserving topic`, e);
        if (e instanceof UnauthorizedError) {
          session.resetAndRedirect(routes.login);
        } else if (e instanceof TopicReservedError) {
          setError(t("subscribe_dialog_error_topic_already_reserved"));
          return;
        }
      }
    }

    console.log(`[SubscribeDialog] Successful login to ${topicUrl(baseUrl, topic)} for user ${username}`);
    props.onSuccess(backgroundNotificationsEnabled);
  };

  const handleUseAnotherChanged = (e) => {
    props.setBaseUrl("");
    setAnotherServerVisible(e.target.checked);
    if (e.target.checked) {
      setBackgroundNotificationsEnabled(false);
    }
  };

  const subscribeButtonEnabled = (() => {
    if (anotherServerVisible) {
      const isExistingTopicUrl = existingTopicUrls.includes(topicUrl(baseUrl, topic));
      return validTopic(topic) && validUrl(baseUrl) && !isExistingTopicUrl;
    }
    const isExistingTopicUrl = existingTopicUrls.includes(topicUrl(config.base_url, topic));
    return validTopic(topic) && !isExistingTopicUrl;
  })();

  const updateBaseUrl = (ev, newVal) => {
    if (validUrl(newVal)) {
      props.setBaseUrl(newVal.replace(/\/$/, "")); // strip trailing slash after https?://
    } else {
      props.setBaseUrl(newVal);
    }
  };

  return (
    <>
      <DialogTitle>{t("subscribe_dialog_subscribe_title")}</DialogTitle>
      <DialogContent>
        <DialogContentText>{t("subscribe_dialog_subscribe_description")}</DialogContentText>
        <div style={{ display: "flex", paddingBottom: "8px" }} role="row">
          <TextField
            autoFocus
            margin="dense"
            id="topic"
            placeholder={t("subscribe_dialog_subscribe_topic_placeholder")}
            value={props.topic}
            onChange={(ev) => props.setTopic(ev.target.value)}
            type="text"
            fullWidth
            variant="standard"
            inputProps={{
              maxLength: 64,
              "aria-label": t("subscribe_dialog_subscribe_topic_placeholder"),
            }}
          />
          <Button
            onClick={() => {
              props.setTopic(randomAlphanumericString(16));
            }}
            style={{ flexShrink: "0", marginTop: "0.5em" }}
          >
            {t("subscribe_dialog_subscribe_button_generate_topic_name")}
          </Button>
        </div>
        {showReserveTopicCheckbox && (
          <FormGroup>
            <FormControlLabel
              variant="standard"
              control={
                <Switch
                  disabled={!reserveTopicEnabled}
                  checked={reserveTopicVisible}
                  onChange={(ev) => setReserveTopicVisible(ev.target.checked)}
                  inputProps={{
                    "aria-label": t("reserve_dialog_checkbox_label"),
                  }}
                />
              }
              label={
                <>
                  {t("reserve_dialog_checkbox_label")}
                  <ReserveLimitChip />
                </>
              }
            />
            {reserveTopicVisible && <ReserveTopicSelect value={everyone} onChange={setEveryone} />}
          </FormGroup>
        )}
        {!reserveTopicVisible && (
          <FormGroup>
            <FormControlLabel
              control={
                <Switch
                  onChange={handleUseAnotherChanged}
                  checked={anotherServerVisible}
                  inputProps={{
                    "aria-label": t("subscribe_dialog_subscribe_use_another_label"),
                  }}
                />
              }
              label={t("subscribe_dialog_subscribe_use_another_label")}
            />
            {anotherServerVisible && (
              <Autocomplete
                freeSolo
                options={existingBaseUrls}
                inputValue={props.baseUrl}
                onInputChange={updateBaseUrl}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    placeholder={config.base_url}
                    variant="standard"
                    aria-label={t("subscribe_dialog_subscribe_base_url_label")}
                  />
                )}
              />
            )}
          </FormGroup>
        )}
        {notifier.pushPossible() && !anotherServerVisible && (
          <FormGroup>
            <FormControlLabel
              control={
                <Switch
                  onChange={handleBackgroundNotificationsChanged}
                  checked={backgroundNotificationsEnabled}
                  inputProps={{
                    "aria-label": t("subscribe_dialog_subscribe_enable_background_notifications_label"),
                  }}
                />
              }
              label={t("subscribe_dialog_subscribe_enable_background_notifications_label")}
            />
          </FormGroup>
        )}
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={props.onCancel}>{t("subscribe_dialog_subscribe_button_cancel")}</Button>
        <Button onClick={handleSubscribe} disabled={!subscribeButtonEnabled}>
          {t("subscribe_dialog_subscribe_button_subscribe")}
        </Button>
      </DialogFooter>
    </>
  );
};

const LoginPage = (props) => {
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const baseUrl = props.baseUrl ? props.baseUrl : config.base_url;
  const { topic } = props;

  const handleLogin = async () => {
    const user = { baseUrl, username, password };
    const success = await api.topicAuth(baseUrl, topic, user);
    if (!success) {
      console.log(`[SubscribeDialog] Login to ${topicUrl(baseUrl, topic)} failed for user ${username}`);
      setError(t("subscribe_dialog_error_user_not_authorized", { username }));
      return;
    }
    console.log(`[SubscribeDialog] Successful login to ${topicUrl(baseUrl, topic)} for user ${username}`);
    await userManager.save(user);
    props.onSuccess();
  };

  return (
    <>
      <DialogTitle>{t("subscribe_dialog_login_title")}</DialogTitle>
      <DialogContent>
        <DialogContentText>{t("subscribe_dialog_login_description")}</DialogContentText>
        <TextField
          autoFocus
          margin="dense"
          id="username"
          label={t("subscribe_dialog_login_username_label")}
          value={username}
          onChange={(ev) => setUsername(ev.target.value)}
          type="text"
          fullWidth
          variant="standard"
          inputProps={{
            "aria-label": t("subscribe_dialog_login_username_label"),
          }}
        />
        <TextField
          margin="dense"
          id="password"
          label={t("subscribe_dialog_login_password_label")}
          type="password"
          value={password}
          onChange={(ev) => setPassword(ev.target.value)}
          fullWidth
          variant="standard"
          inputProps={{
            "aria-label": t("subscribe_dialog_login_password_label"),
          }}
        />
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={props.onBack}>{t("common_back")}</Button>
        <Button onClick={handleLogin}>{t("subscribe_dialog_login_button_login")}</Button>
      </DialogFooter>
    </>
  );
};

export default SubscribeDialog;
