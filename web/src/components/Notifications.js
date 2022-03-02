import Container from "@mui/material/Container";
import {CardContent, Link, Stack} from "@mui/material";
import Card from "@mui/material/Card";
import Typography from "@mui/material/Typography";
import * as React from "react";
import {formatMessage, formatTitle, topicShortUrl, unmatchedTags} from "../app/utils";
import IconButton from "@mui/material/IconButton";
import CloseIcon from '@mui/icons-material/Close';
import {Paragraph, VerticallyCenteredContainer} from "./styles";
import {useLiveQuery} from "dexie-react-hooks";
import db from "../app/db";

const Notifications = (props) => {
    const subscription = props.subscription;
    const notifications = useLiveQuery(() => {
        return db.notifications
            .where({ subscriptionId: subscription.id })
            .toArray();
    }, [subscription]);
    if (!notifications || notifications.length === 0) {
        return <NothingHereYet subscription={subscription}/>;
    }
    const sortedNotifications = Array.from(notifications)
        .sort((a, b) => a.time < b.time ? 1 : -1);
    return (
        <Container maxWidth="lg" sx={{marginTop: 3, marginBottom: 3}}>
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
    const date = new Intl.DateTimeFormat('default', {dateStyle: 'short', timeStyle: 'short'})
        .format(new Date(notification.time * 1000));
    const otherTags = unmatchedTags(notification.tags);
    const tags = (otherTags.length > 0) ? otherTags.join(', ') : null;
    const handleDelete = async () => {
        console.log(`[Notifications] Deleting notification ${notification.id} from ${subscriptionId}`);
        await db.notifications.delete(notification.id); // FIXME
    }
    return (
        <Card sx={{ minWidth: 275 }}>
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
                {tags && <Typography sx={{ fontSize: 14 }} color="text.secondary">Tags: {tags}</Typography>}
            </CardContent>
        </Card>
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
