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
        <AppBar position="fixed" sx={{
            width: '100%',
            zIndex: { sm: 2000 }, // > Navigation
            ml: { sm: `${Navigation.width}px` }
        }}>
            <Toolbar sx={{pr: '24px'}}>
                <IconButton
                    color="inherit"
                    edge="start"
                    onClick={props.onMobileDrawerToggle}
                    sx={{ mr: 2, display: { sm: 'none' } }}
                >
                    <MenuIcon />
                </IconButton>
                <Typography variant="h6" noWrap component="div" sx={{ flexGrow: 1 }}>
                    {title}
                </Typography>
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

/*
    To add a top left corner logo box:
        <Typography variant="h5" noWrap component="div" sx={{
            display: { xs: 'none', sm: 'block' },
            width: { sm: `${Navigation.width}px` }
        }}>
            ntfy
        </Typography>

    To make the size of the top bar dynamic based on the drawer:
        width: { sm: `calc(100% - ${Navigation.width}px)` }
*/

export default ActionBar;
