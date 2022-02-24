import Container from "@mui/material/Container";
import {CardContent, Stack} from "@mui/material";
import Card from "@mui/material/Card";
import Typography from "@mui/material/Typography";
import * as React from "react";
import {formatTitle, formatMessage, unmatchedTags} from "../app/utils";

const NotificationList = (props) => {
    const sortedNotifications = props.notifications.sort((a, b) => a.time < b.time);
    return (
        <Container maxWidth="lg" sx={{ marginTop: 3, marginBottom: 3 }}>
            <Stack spacing={3}>
                {sortedNotifications.map(notification =>
                    <NotificationItem key={notification.id} notification={notification}/>)}
            </Stack>
        </Container>
    );
}

const NotificationItem = (props) => {
    const notification = props.notification;
    const date = new Intl.DateTimeFormat('default', {dateStyle: 'short', timeStyle: 'short'})
        .format(new Date(notification.time * 1000));
    const otherTags = unmatchedTags(notification.tags);
    const tags = (otherTags.length > 0) ? otherTags.join(', ') : null;
    return (
        <Card sx={{ minWidth: 275 }}>
            <CardContent>
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

export default NotificationList;
