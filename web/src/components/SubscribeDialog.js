import * as React from 'react';
import {useContext, useState} from 'react';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import {Autocomplete, Checkbox, Chip, FormControlLabel, FormGroup, useMediaQuery} from "@mui/material";
import theme from "./theme";
import api from "../app/Api";
import {randomAlphanumericString, topicUrl, validTopic, validUrl} from "../app/utils";
import userManager from "../app/UserManager";
import subscriptionManager from "../app/SubscriptionManager";
import poller from "../app/Poller";
import DialogFooter from "./DialogFooter";
import {useTranslation} from "react-i18next";
import session from "../app/Session";
import routes from "./routes";
import accountApi, {Role} from "../app/AccountApi";
import ReserveTopicSelect from "./ReserveTopicSelect";
import {AccountContext} from "./App";
import {TopicReservedError, UnauthorizedError} from "../app/errors";

const publicBaseUrl = "https://ntfy.sh";

const SubscribeDialog = (props) => {
    const [baseUrl, setBaseUrl] = useState("");
    const [topic, setTopic] = useState("");
    const [showLoginPage, setShowLoginPage] = useState(false);
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));

    const handleSuccess = async () => {
        console.log(`[SubscribeDialog] Subscribing to topic ${topic}`);
        const actualBaseUrl = (baseUrl) ? baseUrl : config.base_url;
        const subscription = subscribeTopic(actualBaseUrl, topic);
        poller.pollInBackground(subscription); // Dangle!
        props.onSuccess(subscription);
    }

    return (
        <Dialog open={props.open} onClose={props.onCancel} fullScreen={fullScreen}>
            {!showLoginPage && <SubscribePage
                baseUrl={baseUrl}
                setBaseUrl={setBaseUrl}
                topic={topic}
                setTopic={setTopic}
                subscriptions={props.subscriptions}
                onCancel={props.onCancel}
                onNeedsLogin={() => setShowLoginPage(true)}
                onSuccess={handleSuccess}
            />}
            {showLoginPage && <LoginPage
                baseUrl={baseUrl}
                topic={topic}
                onBack={() => setShowLoginPage(false)}
                onSuccess={handleSuccess}
            />}
        </Dialog>
    );
};

const SubscribePage = (props) => {
    const { t } = useTranslation();
    const { account } = useContext(AccountContext);
    const [error, setError] = useState("");
    const [reserveTopicVisible, setReserveTopicVisible] = useState(false);
    const [anotherServerVisible, setAnotherServerVisible] = useState(false);
    const [everyone, setEveryone] = useState("deny-all");
    const baseUrl = (anotherServerVisible) ? props.baseUrl : config.base_url;
    const topic = props.topic;
    const existingTopicUrls = props.subscriptions.map(s => topicUrl(s.baseUrl, s.topic));
    const existingBaseUrls = Array
        .from(new Set([publicBaseUrl, ...props.subscriptions.map(s => s.baseUrl)]))
        .filter(s => s !== config.base_url);
    const reserveTopicEnabled = session.exists() && account?.role === Role.USER && (account?.stats.reservations_remaining || 0) > 0;

    const handleSubscribe = async () => {
        const user = await userManager.get(baseUrl); // May be undefined
        const username = (user) ? user.username : t("subscribe_dialog_error_user_anonymous");

        // Check read access to topic
        const success = await api.topicAuth(baseUrl, topic, user);
        if (!success) {
            console.log(`[SubscribeDialog] Login to ${topicUrl(baseUrl, topic)} failed for user ${username}`);
            if (user) {
                setError(t("subscribe_dialog_error_user_not_authorized", { username: username }));
                return;
            } else {
                props.onNeedsLogin();
                return;
            }
        }

        // Reserve topic (if requested)
        if (session.exists() && baseUrl === config.base_url && reserveTopicVisible) {
            console.log(`[SubscribeDialog] Reserving topic ${topic} with everyone access ${everyone}`);
            try {
                await accountApi.upsertReservation(topic, everyone);
                // Account sync later after it was added
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
        props.onSuccess();
    };

    const handleUseAnotherChanged = (e) => {
        props.setBaseUrl("");
        setAnotherServerVisible(e.target.checked);
    };

    const subscribeButtonEnabled = (() => {
        if (anotherServerVisible) {
            const isExistingTopicUrl = existingTopicUrls.includes(topicUrl(baseUrl, topic));
            return validTopic(topic) && validUrl(baseUrl) && !isExistingTopicUrl;
        } else {
            const isExistingTopicUrl = existingTopicUrls.includes(topicUrl(config.base_url, topic));
            return validTopic(topic) && !isExistingTopicUrl;
        }
    })();

    const updateBaseUrl = (ev, newVal) => {
        if (validUrl(newVal)) {
          props.setBaseUrl(newVal.replace(/\/$/, '')); // strip trailing slash after https?://
        } else {
          props.setBaseUrl(newVal);
        }
    };

    return (
        <>
            <DialogTitle>{t("subscribe_dialog_subscribe_title")}</DialogTitle>
            <DialogContent>
                <DialogContentText>
                    {t("subscribe_dialog_subscribe_description")}
                </DialogContentText>
                <div style={{display: 'flex', paddingBottom: "8px"}} role="row">
                    <TextField
                        autoFocus
                        margin="dense"
                        id="topic"
                        placeholder={t("subscribe_dialog_subscribe_topic_placeholder")}
                        value={props.topic}
                        onChange={ev => props.setTopic(ev.target.value)}
                        type="text"
                        fullWidth
                        variant="standard"
                        inputProps={{
                            maxLength: 64,
                            "aria-label": t("subscribe_dialog_subscribe_topic_placeholder")
                        }}
                    />
                    <Button onClick={() => {props.setTopic(randomAlphanumericString(16))}} style={{flexShrink: "0", marginTop: "0.5em"}}>
                        {t("subscribe_dialog_subscribe_button_generate_topic_name")}
                    </Button>
                </div>
                {config.enable_reservations && session.exists() && !anotherServerVisible &&
                    <FormGroup>
                        <FormControlLabel
                            variant="standard"
                            control={
                                <Checkbox
                                    fullWidth
                                    disabled={!reserveTopicEnabled}
                                    checked={reserveTopicVisible}
                                    onChange={(ev) => setReserveTopicVisible(ev.target.checked)}
                                    inputProps={{
                                        "aria-label": t("reserve_dialog_checkbox_label")
                                    }}
                                />
                            }
                            label={
                                <>
                                    {t("reserve_dialog_checkbox_label")}
                                    <Chip label={t("action_bar_reservation_limit_reached")} variant="outlined" color="primary" sx={{ opacity: 0.8, borderWidth: "2px", height: "24px", marginLeft: "5px" }}/>
                                </>
                            }
                        />
                        {reserveTopicVisible &&
                            <ReserveTopicSelect
                                value={everyone}
                                onChange={setEveryone}
                            />
                        }
                    </FormGroup>
                }
                {!reserveTopicVisible &&
                    <FormGroup>
                        <FormControlLabel
                            control={
                                <Checkbox
                                    onChange={handleUseAnotherChanged}
                                    inputProps={{
                                        "aria-label": t("subscribe_dialog_subscribe_use_another_label")
                                    }}
                                />
                            }
                            label={t("subscribe_dialog_subscribe_use_another_label")}/>
                        {anotherServerVisible && <Autocomplete
                            freeSolo
                            options={existingBaseUrls}
                            inputValue={props.baseUrl}
                            onInputChange={updateBaseUrl}
                            renderInput={(params) =>
                                <TextField
                                    {...params}
                                    placeholder={config.base_url}
                                    variant="standard"
                                    aria-label={t("subscribe_dialog_subscribe_base_url_label")}
                                />
                            }
                        />}
                    </FormGroup>
                }
            </DialogContent>
            <DialogFooter status={error}>
                <Button onClick={props.onCancel}>{t("subscribe_dialog_subscribe_button_cancel")}</Button>
                <Button onClick={handleSubscribe} disabled={!subscribeButtonEnabled}>{t("subscribe_dialog_subscribe_button_subscribe")}</Button>
            </DialogFooter>
        </>
    );
};

const LoginPage = (props) => {
    const { t } = useTranslation();
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const [error, setError] = useState("");
    const baseUrl = (props.baseUrl) ? props.baseUrl : config.base_url;
    const topic = props.topic;

    const handleLogin = async () => {
        const user = {baseUrl, username, password};
        const success = await api.topicAuth(baseUrl, topic, user);
        if (!success) {
            console.log(`[SubscribeDialog] Login to ${topicUrl(baseUrl, topic)} failed for user ${username}`);
            setError(t("subscribe_dialog_error_user_not_authorized", { username: username }));
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
                <DialogContentText>
                    {t("subscribe_dialog_login_description")}
                </DialogContentText>
                <TextField
                    autoFocus
                    margin="dense"
                    id="username"
                    label={t("subscribe_dialog_login_username_label")}
                    value={username}
                    onChange={ev => setUsername(ev.target.value)}
                    type="text"
                    fullWidth
                    variant="standard"
                    inputProps={{
                        "aria-label": t("subscribe_dialog_login_username_label")
                    }}
                />
                <TextField
                    margin="dense"
                    id="password"
                    label={t("subscribe_dialog_login_password_label")}
                    type="password"
                    value={password}
                    onChange={ev => setPassword(ev.target.value)}
                    fullWidth
                    variant="standard"
                    inputProps={{
                        "aria-label": t("subscribe_dialog_login_password_label")
                    }}
                />
            </DialogContent>
            <DialogFooter status={error}>
                <Button onClick={props.onBack}>{t("subscribe_dialog_login_button_back")}</Button>
                <Button onClick={handleLogin}>{t("subscribe_dialog_login_button_login")}</Button>
            </DialogFooter>
        </>
    );
};

export const subscribeTopic = async (baseUrl, topic) => {
    const subscription = await subscriptionManager.add(baseUrl, topic);
    if (session.exists()) {
        try {
            const remoteSubscription = await accountApi.addSubscription({
                base_url: baseUrl,
                topic: topic
            });
            await subscriptionManager.setRemoteId(subscription.id, remoteSubscription.id);
        } catch (e) {
            console.log(`[SubscribeDialog] Subscribing to topic ${topic} failed`, e);
            if (e instanceof UnauthorizedError) {
                session.resetAndRedirect(routes.login);
            }
        }
    }
    return subscription;
};

export default SubscribeDialog;
