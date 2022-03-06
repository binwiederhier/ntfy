import * as React from 'react';
import {useState} from 'react';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import {Autocomplete, Checkbox, FormControlLabel, useMediaQuery} from "@mui/material";
import theme from "./theme";
import api from "../app/Api";
import config from "../app/config";
import {topicUrl, validTopic, validUrl} from "../app/utils";
import Box from "@mui/material/Box";
import userManager from "../app/UserManager";
import subscriptionManager from "../app/SubscriptionManager";
import poller from "../app/Poller";

const publicBaseUrl = "https://ntfy.sh"

const SubscribeDialog = (props) => {
    const [baseUrl, setBaseUrl] = useState("");
    const [topic, setTopic] = useState("");
    const [showLoginPage, setShowLoginPage] = useState(false);
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const handleSuccess = async () => {
        const actualBaseUrl = (baseUrl) ? baseUrl : config.defaultBaseUrl;
        const subscription = {
            id: topicUrl(actualBaseUrl, topic),
            baseUrl: actualBaseUrl,
            topic: topic,
            last: null
        };
        await subscriptionManager.save(subscription);
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
    const [anotherServerVisible, setAnotherServerVisible] = useState(false);
    const [errorText, setErrorText] = useState("");
    const baseUrl = (anotherServerVisible) ? props.baseUrl : config.defaultBaseUrl;
    const topic = props.topic;
    const existingTopicUrls = props.subscriptions.map(s => topicUrl(s.baseUrl, s.topic));
    const existingBaseUrls = Array.from(new Set([publicBaseUrl, ...props.subscriptions.map(s => s.baseUrl)]))
        .filter(s => s !== config.defaultBaseUrl);
    const handleSubscribe = async () => {
        const user = await userManager.get(baseUrl); // May be undefined
        const username = (user) ? user.username : "anonymous";
        const success = await api.auth(baseUrl, topic, user);
        if (!success) {
            console.log(`[SubscribeDialog] Login to ${topicUrl(baseUrl, topic)} failed for user ${username}`);
            if (user) {
                setErrorText(`User ${username} not authorized`);
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
            const isExistingTopicUrl = existingTopicUrls.includes(topicUrl(config.defaultBaseUrl, topic)); // FIXME
            return validTopic(topic) && !isExistingTopicUrl;
        }
    })();
    return (
        <>
            <DialogTitle>Subscribe to topic</DialogTitle>
            <DialogContent>
                <DialogContentText>
                    Topics may not be password-protected, so choose a name that's not easy to guess.
                    Once subscribed, you can PUT/POST notifications.
                </DialogContentText>
                <TextField
                    autoFocus
                    margin="dense"
                    id="topic"
                    placeholder="Topic name, e.g. phil_alerts"
                    inputProps={{ maxLength: 64 }}
                    value={props.topic}
                    onChange={ev => props.setTopic(ev.target.value)}
                    type="text"
                    fullWidth
                    variant="standard"
                />
                <FormControlLabel
                    sx={{pt: 1}}
                    control={<Checkbox onChange={handleUseAnotherChanged}/>}
                    label="Use another server" />
                {anotherServerVisible && <Autocomplete
                    freeSolo
                    options={existingBaseUrls}
                    sx={{ maxWidth: 400 }}
                    inputValue={props.baseUrl}
                    onInputChange={(ev, newVal) => props.setBaseUrl(newVal)}
                    renderInput={ (params) =>
                        <TextField {...params} placeholder={config.defaultBaseUrl} variant="standard"/>
                    }
                />}
            </DialogContent>
            <DialogFooter status={errorText}>
                <Button onClick={props.onCancel}>Cancel</Button>
                <Button onClick={handleSubscribe} disabled={!subscribeButtonEnabled}>Subscribe</Button>
            </DialogFooter>
        </>
    );
};

const LoginPage = (props) => {
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const [errorText, setErrorText] = useState("");
    const baseUrl = (props.baseUrl) ? props.baseUrl : config.defaultBaseUrl;
    const topic = props.topic;
    const handleLogin = async () => {
        const user = {baseUrl, username, password};
        const success = await api.auth(baseUrl, topic, user);
        if (!success) {
            console.log(`[SubscribeDialog] Login to ${topicUrl(baseUrl, topic)} failed for user ${username}`);
            setErrorText(`User ${username} not authorized`);
            return;
        }
        console.log(`[SubscribeDialog] Successful login to ${topicUrl(baseUrl, topic)} for user ${username}`);
        await userManager.save(user);
        props.onSuccess();
    };
    return (
        <>
            <DialogTitle>Login required</DialogTitle>
            <DialogContent>
                <DialogContentText>
                    This topic is password-protected. Please enter username and
                    password to subscribe.
                </DialogContentText>
                <TextField
                    autoFocus
                    margin="dense"
                    id="username"
                    label="Username, e.g. phil"
                    value={username}
                    onChange={ev => setUsername(ev.target.value)}
                    type="text"
                    fullWidth
                    variant="standard"
                />
                <TextField
                    margin="dense"
                    id="password"
                    label="Password"
                    type="password"
                    value={password}
                    onChange={ev => setPassword(ev.target.value)}
                    fullWidth
                    variant="standard"
                />
            </DialogContent>
            <DialogFooter status={errorText}>
                <Button onClick={props.onBack}>Back</Button>
                <Button onClick={handleLogin}>Login</Button>
            </DialogFooter>
        </>
    );
};

const DialogFooter = (props) => {
    return (
        <Box sx={{
            display: 'flex',
            flexDirection: 'row',
            justifyContent: 'space-between',
            paddingLeft: '24px',
            paddingTop: '8px 24px',
            paddingBottom: '8px 24px',
        }}>
            <DialogContentText sx={{
                margin: '0px',
                paddingTop: '8px',
            }}>
                {props.status}
            </DialogContentText>
            <DialogActions>
                {props.children}
            </DialogActions>
        </Box>
    );
};

export default SubscribeDialog;
