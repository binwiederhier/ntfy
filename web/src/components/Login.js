import * as React from 'react';
import {Avatar, Checkbox, FormControlLabel, Grid, Link} from "@mui/material";
import Typography from "@mui/material/Typography";
import LockOutlinedIcon from '@mui/icons-material/LockOutlined';
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import Box from "@mui/material/Box";
import api from "../app/Api";
import routes from "./routes";
import session from "../app/Session";
import logo from "../img/ntfy2.svg";
import {NavLink} from "react-router-dom";

const Login = () => {
    const handleSubmit = async (event) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        const user = {
            username: data.get('username'),
            password: data.get('password'),
        }
        const token = await api.login("http://localhost:2586"/*window.location.origin*/, user);
        console.log(`[Api] User auth for user ${user.username} successful, token is ${token}`);
        session.store(user.username, token);
        window.location.href = routes.app;
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
                Sign in to your ntfy account
            </Typography>
            <Box component="form" onSubmit={handleSubmit} noValidate sx={{mt: 1, maxWidth: 400}}>
                <TextField
                    margin="dense"
                    required
                    fullWidth
                    id="username"
                    label="Username"
                    name="username"
                    autoFocus
                />
                <TextField
                    margin="dense"
                    required
                    fullWidth
                    name="password"
                    label="Password"
                    type="password"
                    id="password"
                    autoComplete="current-password"
                />
                <Button
                    type="submit"
                    fullWidth
                    variant="contained"
                    sx={{mt: 2, mb: 2}}
                >
                    Sign in
                </Button>
                <Box sx={{width: "100%"}}>
                    <div style={{float: "left"}}><NavLink to={routes.resetPassword} variant="body1">Reset password</NavLink></div>
                    <div style={{float: "right"}}><NavLink to={routes.signup} variant="body1">Sign up</NavLink></div>
                </Box>
            </Box>
        </Box>
    );
}

export default Login;
