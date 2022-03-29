import * as React from 'react';
import {useState} from 'react';
import {NotificationItem} from "./Notifications";
import theme from "./theme";
import {
    Chip,
    FormControl,
    InputAdornment, InputLabel,
    Link,
    ListItemIcon,
    ListItemText,
    Select,
    Tooltip,
    useMediaQuery
} from "@mui/material";
import TextField from "@mui/material/TextField";
import priority1 from "../img/priority-1.svg";
import priority2 from "../img/priority-2.svg";
import priority3 from "../img/priority-3.svg";
import priority4 from "../img/priority-4.svg";
import priority5 from "../img/priority-5.svg";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import DialogActions from "@mui/material/DialogActions";
import Button from "@mui/material/Button";
import Typography from "@mui/material/Typography";
import IconButton from "@mui/material/IconButton";
import InsertEmoticonIcon from '@mui/icons-material/InsertEmoticon';
import {Close} from "@mui/icons-material";
import MenuItem from "@mui/material/MenuItem";

const SendDialog = (props) => {
    const [topicUrl, setTopicUrl] = useState(props.topicUrl);
    const [message, setMessage] = useState(props.message || "");
    const [title, setTitle] = useState("");
    const [tags, setTags] = useState("");
    const [priority, setPriority] = useState(3);
    const [clickUrl, setClickUrl] = useState("");
    const [attachUrl, setAttachUrl] = useState("");
    const [filename, setFilename] = useState("");
    const [email, setEmail] = useState("");
    const [delay, setDelay] = useState("");

    const [showTopicUrl, setShowTopicUrl] = useState(props.topicUrl === "");
    const [showClickUrl, setShowClickUrl] = useState(false);
    const [showAttachUrl, setShowAttachUrl] = useState(false);
    const [showAttachFile, setShowAttachFile] = useState(false);
    const [showEmail, setShowEmail] = useState(false);
    const [showDelay, setShowDelay] = useState(false);

    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const sendButtonEnabled = (() => {
        return true;
    })();
    const handleSubmit = async () => {
        props.onSubmit({
            baseUrl: "xx",
            username: username,
            password: password
        })
    };
    return (
        <Dialog maxWidth="md" open={props.open} onClose={props.onCancel} fullScreen={fullScreen}>
            <DialogTitle>Publish notification</DialogTitle>
            <DialogContent>
                {showTopicUrl &&
                    <ClosableRow onClose={() => {
                        setTopicUrl(props.topicUrl);
                        setShowTopicUrl(false);
                    }}>
                        <TextField
                            margin="dense"
                            label="Topic URL"
                            value={topicUrl}
                            onChange={ev => setTopicUrl(ev.target.value)}
                            type="text"
                            variant="standard"
                            fullWidth
                            required
                        />
                    </ClosableRow>
                }
                <TextField
                    margin="dense"
                    label="Title"
                    value={title}
                    onChange={ev => setTitle(ev.target.value)}
                    type="text"
                    fullWidth
                    variant="standard"
                    placeholder="Notification title, e.g. Disk space alert"
                />
                <TextField
                    margin="dense"
                    label="Message"
                    placeholder="Type the main message body here."
                    value={message}
                    onChange={ev => setMessage(ev.target.value)}
                    type="text"
                    variant="standard"
                    rows={5}
                    fullWidth
                    autoFocus
                    multiline
                />
                <div style={{display: 'flex'}}>
                    <DialogIconButton onClick={() => null}><InsertEmoticonIcon/></DialogIconButton>
                    <TextField
                        margin="dense"
                        label="Tags"
                        placeholder="Comma-separated list of tags, e.g. warning, srv1-backup"
                        value={tags}
                        onChange={ev => setTags(ev.target.value)}
                        type="text"
                        variant="standard"
                        sx={{flexGrow: 1, marginRight: 1}}
                    />
                    <FormControl
                        variant="standard"
                        margin="dense"
                        sx={{minWidth: 120, maxWidth: 200, flexGrow: 1}}
                    >
                        <InputLabel/>
                        <Select
                            label="Priority"
                            margin="dense"
                            value={priority}
                            onChange={(ev) => setPriority(ev.target.value)}
                        >
                            {[1,2,3,4,5].map(priority =>
                                <MenuItem value={priority}>
                                    <div style={{ display: 'flex', alignItems: 'center' }}>
                                        <img src={priorities[priority].file} style={{marginRight: "8px"}}/>
                                        <div>{priorities[priority].label}</div>
                                    </div>
                                </MenuItem>
                            )}
                        </Select>
                    </FormControl>
                </div>
                {showClickUrl &&
                    <ClosableRow onClose={() => {
                        setClickUrl("");
                        setShowClickUrl(false);
                    }}>
                        <TextField
                            margin="dense"
                            label="Click URL"
                            placeholder="URL that is opened when notification is clicked"
                            value={clickUrl}
                            onChange={ev => setClickUrl(ev.target.value)}
                            type="url"
                            fullWidth
                            variant="standard"
                        />
                    </ClosableRow>
                }
                {showEmail &&
                    <ClosableRow onClose={() => {
                        setEmail("");
                        setShowEmail(false);
                    }}>
                        <TextField
                        margin="dense"
                        label="Email"
                        placeholder="Address to forward the message to, e.g. phil@example.com"
                        value={email}
                        onChange={ev => setEmail(ev.target.value)}
                        type="email"
                        variant="standard"
                        fullWidth
                    />
                    </ClosableRow>
                }
                {showAttachUrl && <TextField
                    margin="dense"
                    label="Attachment URL"
                    value={attachUrl}
                    onChange={ev => setAttachUrl(ev.target.value)}
                    type="url"
                    variant="standard"
                    fullWidth
                />}
                {(showAttachFile || showAttachUrl) && <TextField
                    margin="dense"
                    label="Attachment Filename"
                    value={filename}
                    onChange={ev => setFilename(ev.target.value)}
                    type="text"
                    variant="standard"
                    fullWidth
                />}
                {showDelay &&
                    <ClosableRow onClose={() => {
                        setDelay("");
                        setShowDelay(false);
                    }}>
                        <TextField
                            margin="dense"
                            label="Delay"
                            placeholder="Unix timestamp, duration or English natural language"
                            value={delay}
                            onChange={ev => setDelay(ev.target.value)}
                            type="text"
                            variant="standard"
                            fullWidth
                        />
                    </ClosableRow>
                }
                <Typography variant="body1" sx={{marginTop: 2, marginBottom: 1}}>
                    Other features:
                </Typography>
                <div>
                    {!showClickUrl && <Chip clickable label="Click URL" onClick={() => setShowClickUrl(true)} sx={{marginRight: 1}}/>}
                    {!showEmail && <Chip clickable label="Forward to email" onClick={() => setShowEmail(true)} sx={{marginRight: 1}}/>}
                    {!showAttachUrl && <Chip clickable label="Attach file by URL" onClick={() => setShowAttachUrl(true)} sx={{marginRight: 1}}/>}
                    {!showAttachFile && <Chip clickable label="Attach local file" onClick={() => setShowAttachFile(true)} sx={{marginRight: 1}}/>}
                    {!showDelay && <Chip clickable label="Delay delivery" onClick={() => setShowDelay(true)} sx={{marginRight: 1}}/>}
                    {!showTopicUrl && <Chip clickable label="Change topic" onClick={() => setShowTopicUrl(true)} sx={{marginRight: 1}}/>}
                </div>
                <Typography variant="body1" sx={{marginTop: 2, marginBottom: 1}}>
                    For examples and a detailed description of all send features, please
                    refer to the <Link href="/docs">documentation</Link>.
                </Typography>
            </DialogContent>
            <DialogActions>
                <Button onClick={props.onCancel}>Cancel</Button>
                <Button onClick={handleSubmit} disabled={!sendButtonEnabled}>Send</Button>
            </DialogActions>
        </Dialog>
    );
};

const Row = (props) => {
    return (
        <div style={{display: 'flex'}}>
            {props.children}
        </div>
    );
};

const ClosableRow = (props) => {
    return (
        <Row>
            {props.children}
            <DialogIconButton onClick={props.onClose}><Close/></DialogIconButton>
        </Row>
    );
};

const PrioritySelect = () => {
    return (
        <Tooltip title="Message priority">
            <IconButton color="inherit" size="large" sx={{height: "45px", marginTop: "15px"}} onClick={() => setSendDialogOpen(true)}>
                <img src={priority3}/>
            </IconButton>
        </Tooltip>
    );
};

const DialogIconButton = (props) => {
    return (
        <IconButton
            color="inherit"
            size="large"
            edge="start"
            sx={{height: "45px", marginTop: "17px", marginLeft: "6px"}}
            onClick={props.onClick}
        >
            {props.children}
        </IconButton>
    );
};

const priorities = {
    1: { label: "Minimum priority", file: priority1 },
    2: { label: "Low priority", file: priority2 },
    3: { label: "Default priority", file: priority3 },
    4: { label: "High priority", file: priority4 },
    5: { label: "Maximum priority", file: priority5 }
};

export default SendDialog;
