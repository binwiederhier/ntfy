import Drawer from "@mui/material/Drawer";
import * as React from "react";
import {useState} from "react";
import ListItemButton from "@mui/material/ListItemButton";
import ListItemIcon from "@mui/material/ListItemIcon";
import ChatBubbleOutlineIcon from "@mui/icons-material/ChatBubbleOutline";
import ListItemText from "@mui/material/ListItemText";
import Toolbar from "@mui/material/Toolbar";
import Divider from "@mui/material/Divider";
import List from "@mui/material/List";
import SettingsIcon from "@mui/icons-material/Settings";
import HomeIcon from '@mui/icons-material/Home';
import AddIcon from "@mui/icons-material/Add";
import SubscribeDialog from "./SubscribeDialog";
import {Alert, AlertTitle, CircularProgress, ListSubheader} from "@mui/material";
import Button from "@mui/material/Button";
import Typography from "@mui/material/Typography";
import {subscriptionRoute, topicShortUrl, topicUrl} from "../app/utils";
import {ConnectionState} from "../app/Connection";
import {useLocation, useNavigate} from "react-router-dom";

const navWidth = 240;

const Navigation = (props) => {
    const navigationList = <NavList {...props}/>;
    return (
        <>
            {/* Mobile drawer; only shown if menu icon clicked (mobile open) and display is small */}
            <Drawer
                variant="temporary"
                open={props.mobileDrawerOpen}
                onClose={props.onMobileDrawerToggle}
                ModalProps={{ keepMounted: true }} // Better open performance on mobile.
                sx={{
                    display: { xs: 'block', sm: 'none' },
                    '& .MuiDrawer-paper': { boxSizing: 'border-box', width: navWidth },
                }}
            >
                {navigationList}
            </Drawer>
            {/* Big screen drawer; persistent, shown if screen is big */}
            <Drawer
                open
                variant="permanent"
                sx={{
                    display: { xs: 'none', sm: 'block' },
                    '& .MuiDrawer-paper': { boxSizing: 'border-box', width: navWidth },
                }}
            >
                {navigationList}
            </Drawer>
        </>
    );
};
Navigation.width = navWidth;

const NavList = (props) => {
    const navigate = useNavigate();
    const location = useLocation();
    const [subscribeDialogKey, setSubscribeDialogKey] = useState(0);
    const [subscribeDialogOpen, setSubscribeDialogOpen] = useState(false);

    const handleSubscribeReset = () => {
        setSubscribeDialogOpen(false);
        setSubscribeDialogKey(prev => prev+1);
    }

    const handleSubscribeSubmit = (subscription) => {
        console.log(`[Navigation] New subscription: ${subscription.id}`, subscription);
        handleSubscribeReset();
        navigate(subscriptionRoute(subscription));
        props.requestNotificationPermission();
    }

    const showSubscriptionsList = props.subscriptions?.length > 0;
    const showGrantPermissionsBox = props.subscriptions?.length > 0 && !props.notificationsGranted;

    return (
        <>
            <Toolbar sx={{ display: { xs: 'none', sm: 'block' } }}/>
            <List component="nav" sx={{ paddingTop: (showGrantPermissionsBox) ? '0' : '' }}>
                {showGrantPermissionsBox && <PermissionAlert onRequestPermissionClick={props.requestNotificationPermission}/>}
                {showSubscriptionsList &&
                    <>
                        <ListSubheader>Subscribed topics</ListSubheader>
                        <SubscriptionList
                            subscriptions={props.subscriptions}
                            selectedSubscription={props.selectedSubscription}
                        />
                        <Divider sx={{my: 1}}/>
                    </>}
                <ListItemButton onClick={() => navigate("/")} selected={location.pathname === "/"}>
                    <ListItemIcon><HomeIcon/></ListItemIcon>
                    <ListItemText primary="Home"/>
                </ListItemButton>
                <ListItemButton onClick={() => navigate("/settings")} selected={location.pathname === "/settings"}>
                    <ListItemIcon><SettingsIcon/></ListItemIcon>
                    <ListItemText primary="Settings"/>
                </ListItemButton>
                <ListItemButton onClick={() => setSubscribeDialogOpen(true)}>
                    <ListItemIcon><AddIcon/></ListItemIcon>
                    <ListItemText primary="Add subscription"/>
                </ListItemButton>
            </List>
            <SubscribeDialog
                key={`subscribeDialog${subscribeDialogKey}`} // Resets dialog when canceled/closed
                open={subscribeDialogOpen}
                subscriptions={props.subscriptions}
                onCancel={handleSubscribeReset}
                onSuccess={handleSubscribeSubmit}
            />
        </>
    );
};

const SubscriptionList = (props) => {
    const sortedSubscriptions = props.subscriptions.sort( (a, b) => {
        return (topicUrl(a.baseUrl, a.topic) < topicUrl(b.baseUrl, b.topic)) ? -1 : 1;
    });
    return (
        <>
            {sortedSubscriptions.map(subscription =>
                <SubscriptionItem
                    key={subscription.id}
                    subscription={subscription}
                    selected={props.selectedSubscription && props.selectedSubscription.id === subscription.id}
            />)}
        </>
    );
}

const SubscriptionItem = (props) => {
    const navigate = useNavigate();
    const subscription = props.subscription;
    const icon = (subscription.state === ConnectionState.Connecting)
        ? <CircularProgress size="24px"/>
        : <ChatBubbleOutlineIcon/>;
    const label = (subscription.baseUrl === window.location.origin)
        ? subscription.topic
        : topicShortUrl(subscription.baseUrl, subscription.topic);
    return (
        <ListItemButton onClick={() => navigate(subscriptionRoute(subscription))} selected={props.selected}>
            <ListItemIcon>{icon}</ListItemIcon>
            <ListItemText primary={label}/>
        </ListItemButton>
    );
};

const PermissionAlert = (props) => {
    return (
        <>
            <Alert severity="warning" sx={{paddingTop: 2}}>
                <AlertTitle>Notifications are disabled</AlertTitle>
                <Typography gutterBottom>
                    Grant your browser permission to display desktop notifications.
                </Typography>
                <Button
                    sx={{float: 'right'}}
                    color="inherit"
                    size="small"
                    onClick={props.onRequestPermissionClick}
                >
                    Grant now
                </Button>
            </Alert>
            <Divider/>
        </>
    );
};

export default Navigation;
