import * as React from 'react';
import {useState} from 'react';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import {Autocomplete, Checkbox, FormControlLabel, useMediaQuery} from "@mui/material";
import theme from "./theme";
import api from "../app/Api";
import {randomAlphanumericString, topicUrl, validTopic, validUrl} from "../app/utils";
import userManager from "../app/UserManager";
import subscriptionManager from "../app/SubscriptionManager";
import poller from "../app/Poller";
import DialogFooter from "./DialogFooter";
import {useTranslation} from "react-i18next";
import session from "../app/Session";

const publicBaseUrl = "https://ntfy.sh";

const SubscribeDialog = (props) => {
    const [baseUrl, setBaseUrl] = useState("");
    const [topic, setTopic] = useState("");
    const [showLoginPage, setShowLoginPage] = useState(false);
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const handleSuccess = async () => {
        const actualBaseUrl = (baseUrl) ? baseUrl : config.baseUrl;
        const subscription = await subscriptionManager.add(actualBaseUrl, topic);
        if (session.exists()) {
            const remoteSubscription = await api.addAccountSubscription(config.baseUrl, session.token(), {
                base_url: actualBaseUrl,
                topic: topic
            });
            await subscriptionManager.setRemoteId(subscription.id, remoteSubscription.id);
        }
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
    const [anotherServerVisible, setAnotherServerVisible] = useState(false);
    const [errorText, setErrorText] = useState("");
    const baseUrl = (anotherServerVisible) ? props.baseUrl : config.baseUrl;
    const topic = props.topic;
    const existingTopicUrls = props.subscriptions.map(s => topicUrl(s.baseUrl, s.topic));
    const existingBaseUrls = Array.from(new Set([publicBaseUrl, ...props.subscriptions.map(s => s.baseUrl)]))
        .filter(s => s !== config.baseUrl);
    const handleSubscribe = async () => {
        const user = await userManager.get(baseUrl); // May be undefined
        const username = (user) ? user.username : t("subscribe_dialog_error_user_anonymous");
        const success = await api.topicAuth(baseUrl, topic, user);
        if (!success) {
            console.log(`[SubscribeDialog] Login to ${topicUrl(baseUrl, topic)} failed for user ${username}`);
            if (user) {
                setErrorText(t("subscribe_dialog_error_user_not_authorized", { username: username }));
                return;
            } else {
                props.onNeedsLogin();
                return;
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
            const isExistingTopicUrl = existingTopicUrls.includes(topicUrl(config.baseUrl, topic));
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
                <div style={{display: 'flex'}} role="row">
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
                <FormControlLabel
                    sx={{pt: 1}}
                    control={
                        <Checkbox
                            onChange={handleUseAnotherChanged}
                            inputProps={{
                                "aria-label": t("subscribe_dialog_subscribe_use_another_label")
                            }}
                        />
                    }
                    label={t("subscribe_dialog_subscribe_use_another_label")} />
                {anotherServerVisible && <Autocomplete
                    freeSolo
                    options={existingBaseUrls}
                    sx={{ maxWidth: 400 }}
                    inputValue={props.baseUrl}
                    onInputChange={updateBaseUrl}
                    renderInput={ (params) =>
                        <TextField
                            {...params}
                            placeholder={config.baseUrl}
                            variant="standard"
                            aria-label={t("subscribe_dialog_subscribe_base_url_label")}
                        />
                    }
                />}
            </DialogContent>
            <DialogFooter status={errorText}>
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
    const [errorText, setErrorText] = useState("");
    const baseUrl = (props.baseUrl) ? props.baseUrl : config.baseUrl;
    const topic = props.topic;
    const handleLogin = async () => {
        const user = {baseUrl, username, password};
        const success = await api.topicAuth(baseUrl, topic, user);
        if (!success) {
            console.log(`[SubscribeDialog] Login to ${topicUrl(baseUrl, topic)} failed for user ${username}`);
            setErrorText(t("subscribe_dialog_error_user_not_authorized", { username: username }));
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
            <DialogFooter status={errorText}>
                <Button onClick={props.onBack}>{t("subscribe_dialog_login_button_back")}</Button>
                <Button onClick={handleLogin}>{t("subscribe_dialog_login_button_login")}</Button>
            </DialogFooter>
        </>
    );
};

export default SubscribeDialog;
