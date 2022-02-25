import * as React from 'react';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import {useState} from "react";
import Subscription from "../app/Subscription";
import {useMediaQuery} from "@mui/material";
import theme from "./theme";
import api from "../app/Api";
import {topicUrl} from "../app/utils";
import useStyles from "./styles";

const defaultBaseUrl = "http://127.0.0.1"
//const defaultBaseUrl = "https://ntfy.sh"

const SubscribeDialog = (props) => {
    const [baseUrl, setBaseUrl] = useState(defaultBaseUrl); // FIXME
    const [topic, setTopic] = useState("");
    const [user, setUser] = useState(null);
    const [showLoginPage, setShowLoginPage] = useState(false);
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const handleCancel = () => {
        setTopic('');
        props.onCancel();
    }
    const handleSubmit = async () => {
        const success = await api.auth(baseUrl, topic, null);
        if (!success) {
            console.log(`[SubscribeDialog] Login required for ${topicUrl(baseUrl, topic)}`)
            setShowLoginPage(true);
            return;
        }
        const subscription = new Subscription(defaultBaseUrl, topic);
        props.onSubmit(subscription);
        setTopic('');
    }
    return (
        <Dialog open={props.open} onClose={props.onClose} fullScreen={fullScreen}>
            {!showLoginPage && <SubscribePage
                topic={topic}
                setTopic={setTopic}
                onCancel={handleCancel}
                onSubmit={handleSubmit}
            />}
            {showLoginPage && <LoginPage
                baseUrl={baseUrl}
                topic={topic}
                onBack={() => setShowLoginPage(false)}
                onSubmit={handleSubmit}
            />}
        </Dialog>
    );
};

const SubscribePage = (props) => {
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
                    label="Topic name, e.g. phil_alerts"
                    value={props.topic}
                    onChange={ev => props.setTopic(ev.target.value)}
                    type="text"
                    fullWidth
                    variant="standard"
                />
            </DialogContent>
            <DialogActions>
                <Button onClick={props.onCancel}>Cancel</Button>
                <Button onClick={props.onSubmit} disabled={props.topic === ""}>Subscribe</Button>
            </DialogActions>
        </>
    );
};

const LoginPage = (props) => {
    const styles = useStyles();
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const [errorText, setErrorText] = useState("");
    const baseUrl = props.baseUrl;
    const topic = props.topic;
    const handleLogin = async () => {
        const user = {username: username, password: password};
        const success = await api.auth(baseUrl, topic, user);
        if (!success) {
            console.log(`[SubscribeDialog] Login to ${topicUrl(baseUrl, topic)} failed for user ${username}`);
            setErrorText(`User ${username} not authorized`);
            return;
        }
        console.log(`[SubscribeDialog] Login to ${topicUrl(baseUrl, topic)} successful for user ${username}`);
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
            <div className={styles.bottomBar}>
                <DialogContentText className={styles.statusText}>
                    {errorText}
                </DialogContentText>
                <DialogActions>
                    <Button onClick={props.onBack}>Back</Button>
                    <Button onClick={handleLogin}>Login</Button>
                </DialogActions>
            </div>
        </>
    );
};

export default SubscribeDialog;
