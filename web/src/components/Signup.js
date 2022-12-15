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

const Signup = () => {
    const handleSubmit = async (event) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        const username = data.get('username');
        const password = data.get('password');
        const user = {
            username: username,
            password: password
        }; // FIXME omg so awful

        await api.createAccount("http://localhost:2586"/*window.location.origin*/, username, password);
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
                Create a ntfy account
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
                <TextField
                    margin="dense"
                    required
                    fullWidth
                    name="confirm-password"
                    label="Confirm password"
                    type="password"
                    id="confirm-password"
                />
                <Button
                    type="submit"
                    fullWidth
                    variant="contained"
                    sx={{mt: 2, mb: 2}}
                >
                    Sign up
                </Button>
            </Box>
            <Typography sx={{mb: 4}}>
                <NavLink to={routes.login} variant="body1">
                    Already have an account? Sign in
                </NavLink>
            </Typography>
        </Box>
    );
}

export default Signup;
