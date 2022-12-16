import * as React from 'react';
import {Avatar, Link} from "@mui/material";
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import Box from "@mui/material/Box";
import api from "../app/Api";
import routes from "./routes";
import session from "../app/Session";
import logo from "../img/ntfy2.svg";
import Typography from "@mui/material/Typography";
import {NavLink} from "react-router-dom";

const ResetPassword = () => {
    const handleSubmit = async (event) => {
        //
    };

    return (
        <Box
            sx={{
                display: 'flex',
                flexGrow: 1,
                justifyContent: 'center',
                flexDirection: 'column',
                alignContent: 'center',
                alignItems: 'center',
                height: '100vh'
            }}
        >
            <Avatar
                sx={{ m: 2, width: 64, height: 64, borderRadius: 3 }}
                src={logo}
                variant="rounded"
            />
            <Typography sx={{ typography: 'h6' }}>
                Reset password
            </Typography>
            <Box component="form" onSubmit={handleSubmit} noValidate sx={{mt: 1, maxWidth: 400}}>
                <TextField
                    margin="dense"
                    required
                    fullWidth
                    id="email"
                    label="Email"
                    name="email"
                    autoFocus
                />
                <Button
                    type="submit"
                    fullWidth
                    variant="contained"
                    sx={{mt: 2, mb: 2}}
                >
                    Reset password
                </Button>
            </Box>
            <Typography sx={{mb: 4}}>
                <NavLink to={routes.login} variant="body1">
                    &lt; Return to sign in
                </NavLink>
            </Typography>
        </Box>
    );
}

export default ResetPassword;
