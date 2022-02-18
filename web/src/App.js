import * as React from 'react';
import Container from '@mui/material/Container';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import {useEffect, useState} from "react";
import Subscription from './Subscription';
import WsConnection from './WsConnection';

const SubscriptionList = (props) => {
    const subscriptions = props.subscriptions;
    return (
        <div className="subscriptionList">
            {Object.keys(subscriptions).map(id =>
                <SubscriptionItem
                    key={id}
                    subscription={subscriptions[id]}
                    selected={props.selectedSubscription === subscriptions[id]}
                    onClick={() => props.handleSubscriptionClick(id)}
                />)
            }
        </div>
    );
}

const SubscriptionItem = (props) => {
    const subscription = props.subscription;
    return (
        <>
            <div
                onClick={props.onClick}
                style={{ fontWeight: props.selected ? 'bold' : '' }}
            >
                {subscription.shortUrl()}
            </div>
        </>
    );
}

const NotificationList = (props) => {
    return (
        <div className="notificationList">
            {props.notifications.map(notification =>
                <NotificationItem key={notification.id} notification={notification}/>)}
        </div>
    );
}

const NotificationItem = (props) => {
    const notification = props.notification;
    return (
        <>
            <div className="date">{notification.time}</div>
            <div className="message">{notification.message}</div>
        </>
    );
}

const defaultBaseUrl = "https://ntfy.sh"

const SubscriptionAddForm = (props) => {
    const [topic, setTopic] = useState("");
    const handleSubmit = (ev) => {
        ev.preventDefault();
        props.onSubmit(new Subscription(defaultBaseUrl, topic));
        setTopic('');
    }
    return (
        <form onSubmit={handleSubmit}>
            <input
                type="text"
                value={topic}
                onChange={ev => setTopic(ev.target.value)}
                placeholder="Topic name, e.g. phil_alerts"
                required
                />
        </form>
    );
}

const App = () => {
    const [subscriptions, setSubscriptions] = useState({});
    const [selectedSubscription, setSelectedSubscription] = useState(null);
    const [connections, setConnections] = useState({});
    const subscriptionChanged = (subscription) => {
        setSubscriptions(prev => ({...prev, [subscription.id]: subscription})); // Fake-replace
    };
    const addSubscription = (subscription) => {
        const connection = new WsConnection(subscription, subscriptionChanged);
        setSubscriptions(prev => ({...prev, [subscription.id]: subscription}));
        setConnections(prev => ({...prev, [connection.id]: connection}));
        connection.start();
    };
    const handleSubscriptionClick = (subscriptionId) => {
        console.log(`handleSubscriptionClick ${subscriptionId}`)
        setSelectedSubscription(subscriptions[subscriptionId]);
    };
    const notifications = (selectedSubscription !== null) ? selectedSubscription.notifications : [];
    return (
        <Container maxWidth="sm">
            <Box sx={{my: 4}}>
                <Typography variant="h4" component="h1" gutterBottom>
                    ntfy
                </Typography>
                <SubscriptionAddForm onSubmit={addSubscription}/>
                <SubscriptionList
                    subscriptions={subscriptions}
                    selectedSubscription={selectedSubscription}
                    handleSubscriptionClick={handleSubscriptionClick}
                />
                <NotificationList notifications={notifications}/>
            </Box>
        </Container>
    );
}

export default App;
