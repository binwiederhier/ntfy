import * as React from 'react';
import {useState} from 'react';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import Subscription from "../app/Subscription";
import {Autocomplete, Checkbox, FormControlLabel, useMediaQuery} from "@mui/material";
import theme from "./theme";
import api from "../app/Api";
import {topicUrl, validTopic, validUrl} from "../app/utils";
import Box from "@mui/material/Box";
import db from "../app/db";

const defaultBaseUrl = "http://127.0.0.1"
//const defaultBaseUrl = "https://ntfy.sh"

const SubscribeDialog = (props) => {
    const [baseUrl, setBaseUrl] = useState("");
    const [topic, setTopic] = useState("");
    const [showLoginPage, setShowLoginPage] = useState(false);
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const handleSuccess = () => {
        const actualBaseUrl = (baseUrl) ? baseUrl : defaultBaseUrl; // FIXME
        const subscription = new Subscription(actualBaseUrl, topic);
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
    const baseUrl = (anotherServerVisible) ? props.baseUrl : defaultBaseUrl;
    const topic = props.topic;
    const existingTopicUrls = props.subscriptions.map((id, s) => s.url());
    const existingBaseUrls = Array.from(new Set(["https://ntfy.sh", ...props.subscriptions.map((id, s) => s.baseUrl)]))
        .filter(s => s !== defaultBaseUrl);
    const handleSubscribe = async () => {
        const success = await api.auth(baseUrl, topic, null);
        if (!success) {
            console.log(`[SubscribeDialog] Login to ${topicUrl(baseUrl, topic)} failed for anonymous user`);
            props.onNeedsLogin();
            return;
        }
        console.log(`[SubscribeDialog] Successful login to ${topicUrl(baseUrl, topic)} for anonymous user`);
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
            const isExistingTopicUrl = existingTopicUrls.includes(topicUrl(defaultBaseUrl, topic)); // FIXME
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
                        <TextField {...params} placeholder={defaultBaseUrl} variant="standard"/>
                    }
                />}
            </DialogContent>
            <DialogActions>
                <Button onClick={props.onCancel}>Cancel</Button>
                <Button onClick={handleSubscribe} disabled={!subscribeButtonEnabled}>Subscribe</Button>
            </DialogActions>
        </>
    );
};

const LoginPage = (props) => {
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const [errorText, setErrorText] = useState("");
    const baseUrl = (props.baseUrl) ? props.baseUrl : defaultBaseUrl;
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
        db.users.put(user);
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
