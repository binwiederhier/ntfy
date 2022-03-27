import * as React from 'react';
import {useState} from 'react';
import {NotificationItem} from "./Notifications";
import theme from "./theme";
import {Link, Rating, useMediaQuery} from "@mui/material";
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

const priorityFiles = {
    1: priority1,
    2: priority2,
    3: priority3,
    4: priority4,
    5: priority5
};

function IconContainer(props) {
    const { value, ...other } = props;
    return <span {...other}><img src={priorityFiles[value]}/></span>;
}

const PrioritySelect = () => {
    return (
        <Rating
            defaultValue={3}
            IconContainerComponent={IconContainer}
            highlightSelectedOnly
        />
    );
}

const SendDialog = (props) => {
    const [topicUrl, setTopicUrl] = useState(props.topicUrl);
    const [message, setMessage] = useState(props.message || "");
    const [title, setTitle] = useState("");
    const [tags, setTags] = useState("");
    const [click, setClick] = useState("");
    const [email, setEmail] = useState("");
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
        <Dialog open={props.open} onClose={props.onCancel} fullScreen={fullScreen}>
            <DialogTitle>Publish notification</DialogTitle>
            <DialogContent>
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
                <TextField
                    margin="dense"
                    label="Message"
                    value={message}
                    onChange={ev => setMessage(ev.target.value)}
                    type="text"
                    variant="standard"
                    fullWidth
                    required
                    autoFocus
                    multiline
                />
                <TextField
                    margin="dense"
                    label="Title"
                    value={title}
                    onChange={ev => setTitle(ev.target.value)}
                    type="text"
                    fullWidth
                    variant="standard"
                />
                <TextField
                    margin="dense"
                    label="Tags"
                    value={tags}
                    onChange={ev => setTags(ev.target.value)}
                    type="text"
                    fullWidth
                    variant="standard"
                />
                <TextField
                    margin="dense"
                    label="Click URL"
                    value={click}
                    onChange={ev => setClick(ev.target.value)}
                    type="url"
                    fullWidth
                    variant="standard"
                />
                <TextField
                    margin="dense"
                    label="Email"
                    value={email}
                    onChange={ev => setEmail(ev.target.value)}
                    type="email"
                    fullWidth
                    variant="standard"
                />
                <PrioritySelect/>
                <Typography variant="body1">
                    For details on what these fields mean, please check out the
                    {" "}<Link href="/docs">documentation</Link>.
                </Typography>
            </DialogContent>
            <DialogActions>
                <Button onClick={props.onCancel}>Cancel</Button>
                <Button onClick={handleSubmit} disabled={!sendButtonEnabled}>Send</Button>
            </DialogActions>
        </Dialog>
    );
};

export default SendDialog;
