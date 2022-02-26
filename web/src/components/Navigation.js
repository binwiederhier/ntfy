import Drawer from "@mui/material/Drawer";
import * as React from "react";
import ListItemButton from "@mui/material/ListItemButton";
import ListItemIcon from "@mui/material/ListItemIcon";
import ChatBubbleOutlineIcon from "@mui/icons-material/ChatBubbleOutline";
import ListItemText from "@mui/material/ListItemText";
import {useState} from "react";
import Toolbar from "@mui/material/Toolbar";
import Divider from "@mui/material/Divider";
import List from "@mui/material/List";
import SettingsIcon from "@mui/icons-material/Settings";
import AddIcon from "@mui/icons-material/Add";
import SubscribeDialog from "./SubscribeDialog";

const navWidth = 240;

const Navigation = (props) => {
    const navigationList =
        <NavList
            subscriptions={props.subscriptions}
            selectedSubscription={props.selectedSubscription}
            onSubscriptionClick={props.onSubscriptionClick}
            onSubscribeSubmit={props.onSubscribeSubmit}
        />;
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
    const [subscribeDialogKey, setSubscribeDialogKey] = useState(0);
    const [subscribeDialogOpen, setSubscribeDialogOpen] = useState(false);
    const handleSubscribeReset = () => {
        setSubscribeDialogOpen(false);
        setSubscribeDialogKey(prev => prev+1);
    }
    const handleSubscribeSubmit = (subscription, user) => {
        handleSubscribeReset();
        props.onSubscribeSubmit(subscription, user);
    }
    return (
        <>
            <Toolbar />
            {props.subscriptions.size() > 0 &&
                <Divider />}
            <List component="nav">
                <NavSubscriptionList
                    subscriptions={props.subscriptions}
                    selectedSubscription={props.selectedSubscription}
                    onSubscriptionClick={props.onSubscriptionClick}
                />
                <Divider sx={{ my: 1 }} />
                <ListItemButton>
                    <ListItemIcon>
                        <SettingsIcon />
                    </ListItemIcon>
                    <ListItemText primary="Settings" />
                </ListItemButton>
                <ListItemButton onClick={() => setSubscribeDialogOpen(true)}>
                    <ListItemIcon>
                        <AddIcon />
                    </ListItemIcon>
                    <ListItemText primary="Add subscription" />
                </ListItemButton>
            </List>
            <SubscribeDialog
                key={subscribeDialogKey} // Resets dialog when canceled/closed
                open={subscribeDialogOpen}
                onCancel={handleSubscribeReset}
                onSuccess={handleSubscribeSubmit}
            />
        </>
    );
};

const NavSubscriptionList = (props) => {
    const subscriptions = props.subscriptions;
    return (
        <>
            {subscriptions.map((id, subscription) =>
                <NavSubscriptionItem
                    key={id}
                    subscription={subscription}
                    selected={props.selectedSubscription && props.selectedSubscription.id === id}
                    onClick={() => props.onSubscriptionClick(id)}
                />)
            }
        </>
    );
}

const NavSubscriptionItem = (props) => {
    const subscription = props.subscription;
    return (
        <ListItemButton onClick={props.onClick} selected={props.selected}>
            <ListItemIcon><ChatBubbleOutlineIcon /></ListItemIcon>
            <ListItemText primary={subscription.shortUrl()}/>
        </ListItemButton>
    );
}

export default Navigation;
