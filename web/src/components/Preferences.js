import * as React from 'react';
import {CardContent} from "@mui/material";
import Typography from "@mui/material/Typography";
import Card from "@mui/material/Card";

const Preferences = (props) => {
    return (
        <>
            <Typography variant="h5">
                Manage users
            </Typography>
            <Card sx={{ minWidth: 275 }}>
                <CardContent>
                    You may manage users for your protected topics here. Please note that since this is a client
                    application only, username and password are stored in the browser's local storage.
                </CardContent>
            </Card>
        </>
    );
};

export default Preferences;
