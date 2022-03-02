import AppBar from "@mui/material/AppBar";
import Navigation from "./Navigation";
import Toolbar from "@mui/material/Toolbar";
import IconButton from "@mui/material/IconButton";
import MenuIcon from "@mui/icons-material/Menu";
import Typography from "@mui/material/Typography";
import IconSubscribeSettings from "./IconSubscribeSettings";
import * as React from "react";
import Box from "@mui/material/Box";
import {topicShortUrl} from "../app/utils";

const ActionBar = (props) => {
    const title = (props.selectedSubscription)
        ? topicShortUrl(props.selectedSubscription.baseUrl, props.selectedSubscription.topic)
        : "ntfy";
    return (
        <AppBar position="fixed" sx={{
            width: '100%',
            zIndex: { sm: 1250 }, // > Navigation (1200), but < Dialog (1300)
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
                <Box component="img" src="static/img/ntfy.svg" sx={{
                    display: { xs: 'none', sm: 'block' },
                    marginRight: '10px',
                    height: '28px'
                }}/>
                <Typography variant="h6" noWrap component="div" sx={{ flexGrow: 1 }}>
                    {title}
                </Typography>
                {props.selectedSubscription !== null && <IconSubscribeSettings
                    subscription={props.selectedSubscription}
                    onUnsubscribe={props.onUnsubscribe}
                />}
            </Toolbar>
        </AppBar>
    );
};

export default ActionBar;
