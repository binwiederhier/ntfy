import Container from "@mui/material/Container";
import {ButtonBase, CardActions, CardContent, Fade, Link, Modal, Stack} from "@mui/material";
import Card from "@mui/material/Card";
import Typography from "@mui/material/Typography";
import * as React from "react";
import {useState} from "react";
import {
    formatBytes,
    formatMessage,
    formatShortDateTime,
    formatTitle,
    openUrl,
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

const Notifications = (props) => {
    const subscription = props.subscription;
    const notifications = useLiveQuery(() => {
        return subscriptionManager.getNotifications(subscription.id);
    }, [subscription]);
    if (!notifications || notifications.length === 0) {
        return <NothingHereYet subscription={subscription}/>;
    }
    const sortedNotifications = Array.from(notifications)
        .sort((a, b) => a.time < b.time ? 1 : -1);
    return (
        <Container maxWidth="md" sx={{marginTop: 3, marginBottom: 3}}>
            <Stack spacing={3}>
                {sortedNotifications.map(notification =>
                    <NotificationItem
                        key={notification.id}
                        subscriptionId={subscription.id}
                        notification={notification}
                    />)}
            </Stack>
        </Container>
    );
}

const NotificationItem = (props) => {
    const subscriptionId = props.subscriptionId;
    const notification = props.notification;
    const attachment = notification.attachment;
    const date = formatShortDateTime(notification.time);
    const otherTags = unmatchedTags(notification.tags);
    const tags = (otherTags.length > 0) ? otherTags.join(', ') : null;
    const handleDelete = async () => {
        console.log(`[Notifications] Deleting notification ${notification.id} from ${subscriptionId}`);
        await subscriptionManager.deleteNotification(notification.id)
    }
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
                            src={`static/img/priority-${notification.priority}.svg`}
                            alt={`Priority ${notification.priority}`}
                            style={{ verticalAlign: 'bottom' }}
                        />}
                </Typography>
                {notification.title && <Typography variant="h5" component="div">{formatTitle(notification)}</Typography>}
                <Typography variant="body1" sx={{ whiteSpace: 'pre-line' }}>{formatMessage(notification)}</Typography>
                {attachment && <Attachment attachment={attachment}/>}
                {tags && <Typography sx={{ fontSize: 14 }} color="text.secondary">Tags: {tags}</Typography>}
            </CardContent>
            {showActions &&
                <CardActions sx={{paddingTop: 0}}>
                    {showAttachmentActions && <>
                        <Button onClick={() => navigator.clipboard.writeText(attachment.url)}>Copy URL</Button>
                        <Button onClick={() => openUrl(attachment.url)}>Open attachment</Button>
                    </>}
                    {showClickAction && <Button onClick={() => openUrl(notification.click)}>Open link</Button>}
                </CardActions>
            }
        </Card>
    );
}

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
                <Icon type={attachment.type}/>
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
                <Icon type={attachment.type}/>
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
                src={`${props.attachment.url}`}
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
                        src={`${props.attachment.url}`}
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

const Icon = (props) => {
    const type = props.type;
    let imageFile;
    if (!type) {
        imageFile = 'file-document.svg';
    } else if (type.startsWith('image/')) {
        imageFile = 'file-image.svg';
    } else if (type.startsWith('video/')) {
        imageFile = 'file-video.svg';
    } else if (type.startsWith('audio/')) {
        imageFile = 'file-audio.svg';
    } else if (type === "application/vnd.android.package-archive") {
        imageFile = 'file-app.svg';
    } else {
        imageFile = 'file-document.svg';
    }
    return (
        <Box
            component="img"
            src={`static/img/${imageFile}`}
            loading="lazy"
            sx={{
                width: '28px',
                height: '28px'
            }}
        />
    );
}

const NothingHereYet = (props) => {
    const shortUrl = topicShortUrl(props.subscription.baseUrl, props.subscription.topic);
    return (
        <VerticallyCenteredContainer maxWidth="xs">
            <Typography variant="h5" align="center" sx={{ paddingBottom: 1 }}>
                <img src="static/img/ntfy-outline.svg" height="64" width="64" alt="No notifications"/><br />
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

export default Notifications;
