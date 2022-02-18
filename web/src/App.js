import * as React from 'react';
import Container from '@mui/material/Container';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import ProTip from './ProTip';

function Copyright() {
    return (
        <Typography variant="body2" color="text.secondary" align="center">
            {'Copyright © '}
            <Link color="inherit" href="https://mui.com/">
                Your Website
            </Link>{' '}
            {new Date().getFullYear()}
            {'.'}
        </Typography>
    );
}

const topicUrl = (baseUrl, topic) => `${baseUrl}/${topic}`;
const shortUrl = (url) => url.replaceAll(/https?:\/\//g, "");
const shortTopicUrl = (baseUrl, topic) => shortUrl(topicUrl(baseUrl, topic))

function SubscriptionList(props) {
    return (
        <div className="subscriptionList">
            {props.subscriptions.map(subscription => <SubscriptionItem key={topicUrl(subscription.base_url, subscription.topic)} {...subscription}/>)}
        </div>
    );
}

function SubscriptionItem(props) {
    return (
        <div>
            <div>{shortTopicUrl(props.base_url, props.topic)}</div>
        </div>
    );
}

function NotificationList(props) {
    return (
        <div className="notificationList">
            {props.notifications.map(notification => <NotificationItem key={notification.id} {...notification}/>)}
            <div className="date">{props.timestamp}</div>
            <div className="message">{props.message}</div>
        </div>
    );
}

function NotificationItem(props) {
    return (
        <div>
            <div className="date">{props.time}</div>
            <div className="message">{props.message}</div>
        </div>
    );
}

export default function App() {
    const subscriptions = [
        {base_url: "https://ntfy.sh", topic: "mytopic"},
        {base_url: "https://ntfy.sh", topic: "phils_alerts"},
    ];
    const notifications = [
        {id: "qGrfmhp3vK", times: 1645193395, message: "Message 1"},
        {id: "m4YYjfxwyT", times: 1645193428, message: "Message 2"}
    ];
    return (
        <Container maxWidth="sm">
            <Box sx={{my: 4}}>
                <Typography variant="h4" component="h1" gutterBottom>
                    ntfy
                </Typography>
                <SubscriptionList subscriptions={subscriptions}/>
                <NotificationList notifications={notifications}/>
                <ProTip/>
                <Copyright/>
            </Box>
        </Container>
    );
}
