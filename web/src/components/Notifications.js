import Container from "@mui/material/Container";
import {
    ButtonBase,
    CardActions,
    CardContent,
    CircularProgress,
    Fade,
    Link,
    Modal,
    Snackbar,
    Stack,
    Tooltip
} from "@mui/material";
import Card from "@mui/material/Card";
import Typography from "@mui/material/Typography";
import * as React from "react";
import {useEffect, useState} from "react";
import {
    formatBytes,
    formatMessage,
    formatShortDateTime,
    formatTitle,
    openUrl,
    shortUrl,
    topicShortUrl,
    unmatchedTags
} from "../app/utils";
import IconButton from "@mui/material/IconButton";
import CloseIcon from '@mui/icons-material/Close';
import {LightboxBackdrop, Paragraph, VerticallyCenteredContainer} from "./styles";
import {useLiveQuery} from "dexie-react-hooks";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import subscriptionManager from "../app/SubscriptionManager";
import InfiniteScroll from "react-infinite-scroll-component";
import priority1 from "../img/priority-1.svg";
import priority2 from "../img/priority-2.svg";
import priority4 from "../img/priority-4.svg";
import priority5 from "../img/priority-5.svg";
import logoOutline from "../img/ntfy-outline.svg";
import AttachmentIcon from "./AttachmentIcon";

const Notifications = (props) => {
    if (props.mode === "all") {
        return (props.subscriptions) ? <AllSubscriptions subscriptions={props.subscriptions}/> : <Loading/>;
    }
    return (props.subscription) ? <SingleSubscription subscription={props.subscription}/> : <Loading/>;
}

const AllSubscriptions = (props) => {
    const subscriptions = props.subscriptions;
    const notifications = useLiveQuery(() => subscriptionManager.getAllNotifications(), []);
    if (notifications === null || notifications === undefined) {
        return <Loading/>;
    } else if (subscriptions.length === 0) {
        return <NoSubscriptions/>;
    } else if (notifications.length === 0) {
        return <NoNotificationsWithoutSubscription subscriptions={subscriptions}/>;
    }
    return <NotificationList key="all" notifications={notifications} messageBar={false}/>;
}

const SingleSubscription = (props) => {
    const subscription = props.subscription;
    const notifications = useLiveQuery(() => subscriptionManager.getNotifications(subscription.id), [subscription]);
    if (notifications === null || notifications === undefined) {
        return <Loading/>;
    } else if (notifications.length === 0) {
        return <NoNotifications subscription={subscription}/>;
    }
    return <NotificationList id={subscription.id} notifications={notifications} messageBar={true}/>;
}

const NotificationList = (props) => {
    const pageSize = 20;
    const notifications = props.notifications;
    const [snackOpen, setSnackOpen] = useState(false);
    const [maxCount, setMaxCount] = useState(pageSize);
    const count = Math.min(notifications.length, maxCount);

    useEffect(() => {
        return () => {
            setMaxCount(pageSize);
            document.getElementById("main").scrollTo(0, 0);
        }
    }, [props.id]);

    return (
        <InfiniteScroll
            dataLength={count}
            next={() => setMaxCount(prev => prev + pageSize)}
            hasMore={count < notifications.length}
            loader={<>Loading ...</>}
            scrollThreshold={0.7}
            scrollableTarget="main"
        >
            <Container
                maxWidth="md"
                sx={{
                    marginTop: 3,
                    marginBottom: (props.messageBar) ? "100px" : 3 // Hack to avoid hiding notifications behind the message bar
                }}
            >
                <Stack spacing={3}>
                    {notifications.slice(0, count).map(notification =>
                        <NotificationItem
                            key={notification.id}
                            notification={notification}
                            onShowSnack={() => setSnackOpen(true)}
                        />)}
                    <Snackbar
                        open={snackOpen}
                        autoHideDuration={3000}
                        onClose={() => setSnackOpen(false)}
                        message="Copied to clipboard"
                    />
                </Stack>
            </Container>
        </InfiniteScroll>
    );
}

const NotificationItem = (props) => {
    const notification = props.notification;
    const attachment = notification.attachment;
    const date = formatShortDateTime(notification.time);
    const otherTags = unmatchedTags(notification.tags);
    const tags = (otherTags.length > 0) ? otherTags.join(', ') : null;
    const handleDelete = async () => {
        console.log(`[Notifications] Deleting notification ${notification.id}`);
        await subscriptionManager.deleteNotification(notification.id)
    }
    const handleCopy = (s) => {
        navigator.clipboard.writeText(s);
        props.onShowSnack();
    };
    const expired = attachment && attachment.expires && attachment.expires < Date.now()/1000;
    const showAttachmentActions = attachment && !expired;
    const showClickAction = notification.click;
    const showActions = showAttachmentActions || showClickAction;
    return (
        <Card sx={{ minWidth: 275, padding: 1 }}>
            <CardContent>
                <IconButton onClick={handleDelete} sx={{ float: 'right', marginRight: -1, marginTop: -1 }}>
                    <CloseIcon />
                </IconButton>
                <Typography sx={{ fontSize: 14 }} color="text.secondary">
                    {date}
                    {[1,2,4,5].includes(notification.priority) &&
                        <img
                            src={priorityFiles[notification.priority]}
                            alt={`Priority ${notification.priority}`}
                            style={{ verticalAlign: 'bottom' }}
                        />}
                    {notification.new === 1 &&
                        <svg style={{ width: '8px', height: '8px', marginLeft: '4px' }} viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg">
                            <circle cx="50" cy="50" r="50" fill="#338574"/>
                        </svg>}
                </Typography>
                {notification.title && <Typography variant="h5" component="div">{formatTitle(notification)}</Typography>}
                <Typography variant="body1" sx={{ whiteSpace: 'pre-line' }}>{autolink(formatMessage(notification))}</Typography>
                {attachment && <Attachment attachment={attachment}/>}
                {tags && <Typography sx={{ fontSize: 14 }} color="text.secondary">Tags: {tags}</Typography>}
            </CardContent>
            {showActions &&
                <CardActions sx={{paddingTop: 0}}>
                    {showAttachmentActions && <>
                        <Tooltip title="Copy attachment URL to clipboard">
                            <Button onClick={() => handleCopy(attachment.url)}>Copy URL</Button>
                        </Tooltip>
                        <Tooltip title={`Go to ${attachment.url}`}>
                            <Button onClick={() => openUrl(attachment.url)}>Open attachment</Button>
                        </Tooltip>
                    </>}
                    {showClickAction && <>
                        <Tooltip title="Copy link URL to clipboard">
                            <Button onClick={() => handleCopy(notification.click)}>Copy link</Button>
                        </Tooltip>
                        <Tooltip title={`Go to ${notification.click}`}>
                            <Button onClick={() => openUrl(notification.click)}>Open link</Button>
                        </Tooltip>
                    </>}
                </CardActions>}
        </Card>
    );
}

/**
 * Replace links with <Link/> components; this is a combination of the genius function
 * in [1] and the regex in [2].
 *
 * [1] https://github.com/facebook/react/issues/3386#issuecomment-78605760
 * [2] https://github.com/bryanwoods/autolink-js/blob/master/autolink.js#L9
 */
const autolink = (s) => {
    const parts = s.split(/(\bhttps?:\/\/[\-A-Z0-9+\u0026\u2019@#\/%?=()~_|!:,.;]*[\-A-Z0-9+\u0026@#\/%=~()_|]\b)/gi);
    for (let i = 1; i < parts.length; i += 2) {
        parts[i] = <Link key={i} href={parts[i]} underline="hover" target="_blank" rel="noreferrer,noopener">{shortUrl(parts[i])}</Link>;
    }
    return <>{parts}</>;
};

const priorityFiles = {
    1: priority1,
    2: priority2,
    4: priority4,
    5: priority5
};

const Attachment = (props) => {
    const attachment = props.attachment;
    const expired = attachment.expires && attachment.expires < Date.now()/1000;
    const expires = attachment.expires && attachment.expires > Date.now()/1000;
    const displayableImage = !expired && attachment.type && attachment.type.startsWith("image/");

    // Unexpired image
    if (displayableImage) {
        return <Image attachment={attachment}/>;
    }

    // Anything else: Show box
    const infos = [];
    if (attachment.size) {
        infos.push(formatBytes(attachment.size));
    }
    if (expires) {
        infos.push(`link expires ${formatShortDateTime(attachment.expires)}`);
    }
    if (expired) {
        infos.push(`download link expired`);
    }
    const maybeInfoText = (infos.length > 0) ? <><br/>{infos.join(", ")}</> : null;

    // If expired, just show infos without click target
    if (expired) {
        return (
            <Box sx={{
                    display: 'flex',
                    alignItems: 'center',
                    marginTop: 2,
                    padding: 1,
                    borderRadius: '4px',
            }}>
                <AttachmentIcon type={attachment.type}/>
                <Typography variant="body2" sx={{ marginLeft: 1, textAlign: 'left', color: 'text.primary' }}>
                    <b>{attachment.name}</b>
                    {maybeInfoText}
                </Typography>
            </Box>
        );
    }

    // Not expired
    return (
        <ButtonBase sx={{
            marginTop: 2,
        }}>
            <Link
                href={attachment.url}
                target="_blank"
                rel="noopener"
                underline="none"
                sx={{
                        display: 'flex',
                        alignItems: 'center',
                        padding: 1,
                        borderRadius: '4px',
                        '&:hover': {
                            backgroundColor: 'rgba(0, 0, 0, 0.05)'
                        }
                }}
            >
                <AttachmentIcon type={attachment.type}/>
                <Typography variant="body2" sx={{ marginLeft: 1, textAlign: 'left', color: 'text.primary' }}>
                    <b>{attachment.name}</b>
                    {maybeInfoText}
                </Typography>
            </Link>
        </ButtonBase>
    );
};

const Image = (props) => {
    const [open, setOpen] = useState(false);
    return (
        <>
            <Box
                component="img"
                src={props.attachment.url}
                loading="lazy"
                onClick={() => setOpen(true)}
                sx={{
                    marginTop: 2,
                    borderRadius: '4px',
                    boxShadow: 2,
                    width: 1,
                    maxHeight: '400px',
                    objectFit: 'cover',
                    cursor: 'pointer'
                }}
            />
            <Modal
                open={open}
                onClose={() => setOpen(false)}
                BackdropComponent={LightboxBackdrop}
            >
                <Fade in={open}>
                    <Box
                        component="img"
                        src={props.attachment.url}
                        loading="lazy"
                        sx={{
                            maxWidth: 1,
                            maxHeight: 1,
                            position: 'absolute',
                            top: '50%',
                            left: '50%',
                            transform: 'translate(-50%, -50%)',
                            padding: 4,
                        }}
                    />
                </Fade>
            </Modal>
        </>
    );
}

const NoNotifications = (props) => {
    const shortUrl = topicShortUrl(props.subscription.baseUrl, props.subscription.topic);
    return (
        <VerticallyCenteredContainer maxWidth="xs">
            <Typography variant="h5" align="center" sx={{ paddingBottom: 1 }}>
                <img src={logoOutline} height="64" width="64" alt="No notifications"/><br />
                You haven't received any notifications for this topic yet.
            </Typography>
            <Paragraph>
                To send notifications to this topic, simply PUT or POST to the topic URL.
            </Paragraph>
            <Paragraph>
                Example:<br/>
                <tt>
                    $ curl -d "Hi" {shortUrl}
                </tt>
            </Paragraph>
            <Paragraph>
                For more detailed instructions, check out the <Link href="https://ntfy.sh" target="_blank" rel="noopener">website</Link> or
                {" "}<Link href="https://ntfy.sh/docs" target="_blank" rel="noopener">documentation</Link>.
            </Paragraph>
        </VerticallyCenteredContainer>
    );
};

const NoNotificationsWithoutSubscription = (props) => {
    const subscription = props.subscriptions[0];
    const shortUrl = topicShortUrl(subscription.baseUrl, subscription.topic);
    return (
        <VerticallyCenteredContainer maxWidth="xs">
            <Typography variant="h5" align="center" sx={{ paddingBottom: 1 }}>
                <img src={logoOutline} height="64" width="64" alt="No notifications"/><br />
                You haven't received any notifications.
            </Typography>
            <Paragraph>
                To send notifications to a topic, simply PUT or POST to the topic URL. Here's
                an example using one of your topics.
            </Paragraph>
            <Paragraph>
                Example:<br/>
                <tt>
                    $ curl -d "Hi" {shortUrl}
                </tt>
            </Paragraph>
            <Paragraph>
                For more detailed instructions, check out the <Link href="https://ntfy.sh" target="_blank" rel="noopener">website</Link> or
                {" "}<Link href="https://ntfy.sh/docs" target="_blank" rel="noopener">documentation</Link>.
            </Paragraph>
        </VerticallyCenteredContainer>
    );
};

const NoSubscriptions = () => {
    return (
        <VerticallyCenteredContainer maxWidth="xs">
            <Typography variant="h5" align="center" sx={{ paddingBottom: 1 }}>
                <img src={logoOutline} height="64" width="64" alt="No topics"/><br />
                It looks like you don't have any subscriptions yet.
            </Typography>
            <Paragraph>
                Click the "Add subscription" link to create or subscribe to a topic. After that, you can send messages
                via PUT or POST and you'll receive notifications here.
            </Paragraph>
            <Paragraph>
                For more information, check out the <Link href="https://ntfy.sh" target="_blank" rel="noopener">website</Link> or
                {" "}<Link href="https://ntfy.sh/docs" target="_blank" rel="noopener">documentation</Link>.
            </Paragraph>
        </VerticallyCenteredContainer>
    );
};

const Loading = () => {
    return (
        <VerticallyCenteredContainer>
            <Typography variant="h5" color="text.secondary" align="center" sx={{ paddingBottom: 1 }}>
                <CircularProgress disableShrink sx={{marginBottom: 1}}/><br />
                Loading notifications ...
            </Typography>
        </VerticallyCenteredContainer>
    );
};

export default Notifications;
