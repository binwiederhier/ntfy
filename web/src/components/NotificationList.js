import Container from "@mui/material/Container";
import {CardContent, Stack} from "@mui/material";
import Card from "@mui/material/Card";
import Typography from "@mui/material/Typography";
import * as React from "react";

const NotificationList = (props) => {
    const sortedNotifications = props.notifications.sort((a, b) => a.time < b.time);
    return (
        <Container maxWidth="lg" sx={{ marginTop: 3 }}>
            <Stack container spacing={3}>
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
    const tags = (notification.tags && notification.tags.length > 0) ? notification.tags.join(', ') : null;
    return (
        <Card sx={{ minWidth: 275 }}>
            <CardContent>
                <Typography sx={{ fontSize: 14 }} color="text.secondary">{date}</Typography>
                {notification.title && <Typography variant="h5" component="div">{notification.title}</Typography>}
                <Typography variant="body1" gutterBottom>{notification.message}</Typography>
                {tags && <Typography sx={{ fontSize: 14 }} color="text.secondary">Tags: {tags}</Typography>}
            </CardContent>
        </Card>
    );
}

export default NotificationList;
