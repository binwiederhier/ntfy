import * as React from 'react';
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import Box from "@mui/material/Box";
import api from "../app/Api";
import routes from "./routes";
import session from "../app/Session";
import Typography from "@mui/material/Typography";
import {NavLink} from "react-router-dom";
import AvatarBox from "./AvatarBox";
import {useTranslation} from "react-i18next";

const Signup = () => {
    const { t } = useTranslation();
    const handleSubmit = async (event) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        const user = {
            username: data.get('username'),
            password: data.get('password')
        };
        await api.createAccount(config.baseUrl, user.username, user.password);
        const token = await api.login(config.baseUrl, user);
        console.log(`[Api] User auth for user ${user.username} successful, token is ${token}`);
        session.store(user.username, token);
        window.location.href = routes.app;
    };
    if (!config.enableSignup) {
        return (
            <AvatarBox>
                <Typography sx={{ typography: 'h6' }}>{t("Signup is disabled")}</Typography>
            </AvatarBox>
        );
    }
    return (
        <AvatarBox>
            <Typography sx={{ typography: 'h6' }}>
                {t("Create a ntfy account")}
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
                    {t("Sign up")}
                </Button>
            </Box>
            {config.enableLogin &&
                <Typography sx={{mb: 4}}>
                    <NavLink to={routes.login} variant="body1">
                        {t("Already have an account? Sign in!")}
                    </NavLink>
                </Typography>
            }
        </AvatarBox>
    );
}

export default Signup;
