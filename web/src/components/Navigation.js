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
import AddIcon from "@mui/icons-material/Add";
import SubscribeDialog from "./SubscribeDialog";
import {Alert, AlertTitle, Badge, CircularProgress, ListSubheader} from "@mui/material";
import Button from "@mui/material/Button";
import Typography from "@mui/material/Typography";
import {topicShortUrl, topicUrl} from "../app/utils";
import routes from "./routes";
import {ConnectionState} from "../app/Connection";
import {useLocation, useNavigate} from "react-router-dom";
import subscriptionManager from "../app/SubscriptionManager";
import {ChatBubble, NotificationsOffOutlined} from "@mui/icons-material";
import Box from "@mui/material/Box";
import notifier from "../app/Notifier";
import config from "../app/config";

const navWidth = 280;

const Navigation = (props) => {
    const navigationList = <NavList {...props}/>;
    return (
        <Box component="nav" sx={{width: {sm: Navigation.width}, flexShrink: {sm: 0}}}>
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
        </Box>
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
        navigate(routes.forSubscription(subscription));
        handleRequestNotificationPermission();
    }

    const handleRequestNotificationPermission = () => {
       notifier.maybeRequestPermission(granted => props.onNotificationGranted(granted))
    };

    const showSubscriptionsList = props.subscriptions?.length > 0;
    const showNotificationNotSupportedBox = !notifier.supported();
    const showNotificationGrantBox = notifier.supported() && props.subscriptions?.length > 0 && !props.notificationsGranted;

    return (
        <>
            <Toolbar sx={{ display: { xs: 'none', sm: 'block' } }}/>
            <List component="nav" sx={{ paddingTop: (showNotificationGrantBox || showNotificationNotSupportedBox) ? '0' : '' }}>
                {showNotificationNotSupportedBox && <NotificationNotSupportedAlert/>}
                {showNotificationGrantBox && <NotificationGrantAlert onRequestPermissionClick={handleRequestNotificationPermission}/>}
                {!showSubscriptionsList &&
                    <ListItemButton onClick={() => navigate(routes.root)} selected={location.pathname === config.appRoot}>
                        <ListItemIcon><ChatBubble/></ListItemIcon>
                        <ListItemText primary="All notifications"/>
                    </ListItemButton>}
                {showSubscriptionsList &&
                    <>
                        <ListSubheader>Subscribed topics</ListSubheader>
                        <ListItemButton onClick={() => navigate(routes.root)} selected={location.pathname === config.appRoot}>
                            <ListItemIcon><ChatBubble/></ListItemIcon>
                            <ListItemText primary="All notifications"/>
                        </ListItemButton>
                        <SubscriptionList
                            subscriptions={props.subscriptions}
                            selectedSubscription={props.selectedSubscription}
                        />
                        <Divider sx={{my: 1}}/>
                    </>}
                <ListItemButton onClick={() => navigate(routes.settings)} selected={location.pathname === routes.settings}>
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
    const iconBadge = (subscription.new <= 99) ? subscription.new : "99+";
    const icon = (subscription.state === ConnectionState.Connecting)
        ? <CircularProgress size="24px"/>
        : <Badge badgeContent={iconBadge} invisible={subscription.new === 0} color="primary"><ChatBubbleOutlineIcon/></Badge>;
    const label = (subscription.baseUrl === window.location.origin)
        ? subscription.topic
        : topicShortUrl(subscription.baseUrl, subscription.topic);
    const handleClick = async () => {
        navigate(routes.forSubscription(subscription));
        await subscriptionManager.markNotificationsRead(subscription.id);
    };
    return (
        <ListItemButton onClick={handleClick} selected={props.selected}>
            <ListItemIcon>{icon}</ListItemIcon>
            <ListItemText primary={label}/>
            {subscription.mutedUntil > 0 &&
                <ListItemIcon edge="end"><NotificationsOffOutlined /></ListItemIcon>}
        </ListItemButton>
    );
};

const NotificationGrantAlert = (props) => {
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

const NotificationNotSupportedAlert = () => {
    return (
        <>
            <Alert severity="warning" sx={{paddingTop: 2}}>
                <AlertTitle>Notifications not supported</AlertTitle>
                <Typography gutterBottom>
                    Notifications are not supported in your browser.
                </Typography>
            </Alert>
            <Divider/>
        </>
    );
};

export default Navigation;
