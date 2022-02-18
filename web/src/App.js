import * as React from 'react';
import Container from '@mui/material/Container';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import {useState} from "react";
import Subscription from './Subscription';
import WsConnection from './WsConnection';

function SubscriptionList(props) {
    return (
        <div className="subscriptionList">
            {props.subscriptions.map(subscription =>
                <SubscriptionItem key={subscription.url} subscription={subscription}/>)}
        </div>
    );
}

function SubscriptionItem(props) {
    const subscription = props.subscription;
    return (
        <div>
            <div>{subscription.shortUrl()}</div>
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

const NotificationItem = (props) => {
    return (
        <div>
            <div className="date">{props.time}</div>
            <div className="message">{props.message}</div>
        </div>
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
    const [state, setState] = useState({
        subscriptions: [],
    });
    const notifications = [
        {id: "qGrfmhp3vK", times: 1645193395, message: "Message 1"},
        {id: "m4YYjfxwyT", times: 1645193428, message: "Message 2"}
    ];
    const addSubscription = (newSubscription) => {
        const connection = new WsConnection(newSubscription.wsUrl());
        connection.start();
        setState(prevState => ({
            subscriptions: [...prevState.subscriptions, newSubscription],
        }));
    }
    return (
        <Container maxWidth="sm">
            <Box sx={{my: 4}}>
                <Typography variant="h4" component="h1" gutterBottom>
                    ntfy
                </Typography>
                <SubscriptionAddForm onSubmit={addSubscription}/>
                <SubscriptionList subscriptions={state.subscriptions}/>
                <NotificationList notifications={notifications}/>
            </Box>
        </Container>
    );
}

export default App;
