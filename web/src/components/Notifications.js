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
    formatTitle, maybeAppendActionErrors,
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
import {Trans, useTranslation} from "react-i18next";

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
    const { t } = useTranslation();
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
                        message={t("notifications_copied_to_clipboard")}
                    />
                </Stack>
            </Container>
        </InfiniteScroll>
    );
}

const NotificationItem = (props) => {
    const { t } = useTranslation();
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
    const hasAttachmentActions = attachment && !expired;
    const hasClickAction = notification.click;
    const hasUserActions = notification.actions && notification.actions.length > 0;
    const showActions = hasAttachmentActions || hasClickAction || hasUserActions;
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
                <Typography variant="body1" sx={{ whiteSpace: 'pre-line' }}>
                    {autolink(maybeAppendActionErrors(formatMessage(notification), notification))}
                </Typography>
                {attachment && <Attachment attachment={attachment}/>}
                {tags && <Typography sx={{ fontSize: 14 }} color="text.secondary">{t("notifications_tags")}: {tags}</Typography>}
            </CardContent>
            {showActions &&
                <CardActions sx={{paddingTop: 0}}>
                    {hasAttachmentActions && <>
                        <Tooltip title={t("notifications_attachment_copy_url_title")}>
                            <Button onClick={() => handleCopy(attachment.url)}>{t("notifications_attachment_copy_url_button")}</Button>
                        </Tooltip>
                        <Tooltip title={t("notifications_attachment_open_title", { url: attachment.url })}>
                            <Button onClick={() => openUrl(attachment.url)}>{t("notifications_attachment_open_button")}</Button>
                        </Tooltip>
                    </>}
                    {hasClickAction && <>
                        <Tooltip title={t("notifications_click_copy_url_title")}>
                            <Button onClick={() => handleCopy(notification.click)}>{t("notifications_click_copy_url_button")}</Button>
                        </Tooltip>
                        <Tooltip title={t("notifications_actions_open_url_title", { url: notification.click })}>
                            <Button onClick={() => openUrl(notification.click)}>{t("notifications_click_open_button")}</Button>
                        </Tooltip>
                    </>}
                    {hasUserActions && <UserActions notification={notification}/>}
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
    const { t } = useTranslation();
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
        infos.push(t("notifications_attachment_link_expires", { date: formatShortDateTime(attachment.expires) }));
    }
    if (expired) {
        infos.push(t("notifications_attachment_link_expired"));
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

const UserActions = (props) => {
    return (
        <>{props.notification.actions.map(action =>
            <UserAction key={action.id} notification={props.notification} action={action}/>)}</>
    );
};

const UserAction = (props) => {
    const { t } = useTranslation();
    const notification = props.notification;
    const action = props.action;
    if (action.action === "broadcast") {
        return (
            <Tooltip title={t("notifications_actions_not_supported")}>
                <span><Button disabled>{action.label}</Button></span>
            </Tooltip>
        );
    } else if (action.action === "view") {
        return (
            <Tooltip title={t("notifications_actions_open_url_title", { url: action.url })}>
                <Button onClick={() => openUrl(action.url)}>{action.label}</Button>
            </Tooltip>
        );
    } else if (action.action === "http") {
        const method = action.method ?? "POST";
        const label = action.label + (ACTION_LABEL_SUFFIX[action.progress ?? 0] ?? "");
        return (
            <Tooltip title={t("notifications_actions_http_request_title", { method: method, url: action.url })}>
                <Button onClick={() => performHttpAction(notification, action)}>{label}</Button>
            </Tooltip>
        );
    }
    return null; // Others
};

const performHttpAction = async (notification, action) => {
    console.log(`[Notifications] Performing HTTP user action`, action);
    try {
        updateActionStatus(notification, action, ACTION_PROGRESS_ONGOING, null);
        const response = await fetch(action.url, {
            method: action.method ?? "POST",
            headers: action.headers ?? {},
            body: action.body ?? ""
        });
        console.log(`[Notifications] HTTP user action response`, response);
        const success = response.status >= 200 && response.status <= 299;
        if (success) {
            updateActionStatus(notification, action, ACTION_PROGRESS_SUCCESS, null);
        } else {
            updateActionStatus(notification, action, ACTION_PROGRESS_FAILED, `${action.label}: Unexpected response HTTP ${response.status}`);
        }
    } catch (e) {
        console.log(`[Notifications] HTTP action failed`, e);
        updateActionStatus(notification, action, ACTION_PROGRESS_FAILED, `${action.label}: ${e} Check developer console for details.`);
    }
};

const updateActionStatus = (notification, action, progress, error) => {
    notification.actions = notification.actions.map(a => {
        if (a.id !== action.id) {
            return a;
        }
        return { ...a, progress: progress, error: error };
    });
    subscriptionManager.updateNotification(notification);
}

const ACTION_PROGRESS_ONGOING = 1;
const ACTION_PROGRESS_SUCCESS = 2;
const ACTION_PROGRESS_FAILED = 3;

const ACTION_LABEL_SUFFIX = {
    [ACTION_PROGRESS_ONGOING]: " …",
    [ACTION_PROGRESS_SUCCESS]: " ✔",
    [ACTION_PROGRESS_FAILED]: " ❌"
};

const NoNotifications = (props) => {
    const { t } = useTranslation();
    const shortUrl = topicShortUrl(props.subscription.baseUrl, props.subscription.topic);
    return (
        <VerticallyCenteredContainer maxWidth="xs">
            <Typography variant="h5" align="center" sx={{ paddingBottom: 1 }}>
                <img src={logoOutline} height="64" width="64"/><br />
                {t("notifications_none_for_topic_title")}
            </Typography>
            <Paragraph>
                {t("notifications_none_for_topic_description")}
            </Paragraph>
            <Paragraph>
                {t("notifications_example")}:<br/>
                <tt>
                    $ curl -d "Hi" {shortUrl}
                </tt>
            </Paragraph>
            <Paragraph>
                <ForMoreDetails/>
            </Paragraph>
        </VerticallyCenteredContainer>
    );
};

const NoNotificationsWithoutSubscription = (props) => {
    const { t } = useTranslation();
    const subscription = props.subscriptions[0];
    const shortUrl = topicShortUrl(subscription.baseUrl, subscription.topic);
    return (
        <VerticallyCenteredContainer maxWidth="xs">
            <Typography variant="h5" align="center" sx={{ paddingBottom: 1 }}>
                <img src={logoOutline} height="64" width="64"/><br />
                {t("notifications_none_for_any_title")}
            </Typography>
            <Paragraph>
                {t("notifications_none_for_any_description")}
            </Paragraph>
            <Paragraph>
                {t("notifications_example")}:<br/>
                <tt>
                    $ curl -d "Hi" {shortUrl}
                </tt>
            </Paragraph>
            <Paragraph>
                <ForMoreDetails/>
            </Paragraph>
        </VerticallyCenteredContainer>
    );
};

const NoSubscriptions = () => {
    const { t } = useTranslation();
    return (
        <VerticallyCenteredContainer maxWidth="xs">
            <Typography variant="h5" align="center" sx={{ paddingBottom: 1 }}>
                <img src={logoOutline} height="64" width="64"/><br />
                {t("notifications_no_subscriptions_title")}
            </Typography>
            <Paragraph>
                {t("notifications_no_subscriptions_description", {
                    linktext: t("nav_button_subscribe")
                })}
            </Paragraph>
            <Paragraph>
                <ForMoreDetails/>
            </Paragraph>
        </VerticallyCenteredContainer>
    );
};

const ForMoreDetails = () => {
    return (
        <Trans
            i18nKey="notifications_more_details"
            components={{
                websiteLink: <Link href="https://ntfy.sh" target="_blank" rel="noopener"/>,
                docsLink: <Link href="https://ntfy.sh/docs" target="_blank" rel="noopener"/>
            }}
        />
    );
};

const Loading = () => {
    const { t } = useTranslation();
    return (
        <VerticallyCenteredContainer>
            <Typography variant="h5" color="text.secondary" align="center" sx={{ paddingBottom: 1 }}>
                <CircularProgress disableShrink sx={{marginBottom: 1}}/><br />
                {t("notifications_loading")}
            </Typography>
        </VerticallyCenteredContainer>
    );
};

export default Notifications;
