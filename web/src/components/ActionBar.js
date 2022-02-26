import AppBar from "@mui/material/AppBar";
import Navigation from "./Navigation";
import Toolbar from "@mui/material/Toolbar";
import IconButton from "@mui/material/IconButton";
import MenuIcon from "@mui/icons-material/Menu";
import Typography from "@mui/material/Typography";
import IconSubscribeSettings from "./IconSubscribeSettings";
import * as React from "react";

const ActionBar = (props) => {
    const title = (props.selectedSubscription !== null)
        ? props.selectedSubscription.shortUrl()
        : "ntfy";
    return (
        <AppBar position="fixed" sx={{ width: { sm: `calc(100% - ${Navigation.width}px)` }, ml: { sm: `${Navigation.width}px` } }}>
            <Toolbar sx={{pr: '24px'}}>
                <IconButton
                    color="inherit"
                    edge="start"
                    onClick={props.onMobileDrawerToggle}
                    sx={{ mr: 2, display: { sm: 'none' } }}
                >
                    <MenuIcon />
                </IconButton>
                <Typography variant="h6" noWrap component="div" sx={{ flexGrow: 1 }}>{title}</Typography>
                {props.selectedSubscription !== null && <IconSubscribeSettings
                    subscription={props.selectedSubscription}
                    users={props.users}
                    onClearAll={props.onClearAll}
                    onUnsubscribe={props.onUnsubscribe}
                />}
            </Toolbar>
        </AppBar>
    );
};

export default ActionBar;
