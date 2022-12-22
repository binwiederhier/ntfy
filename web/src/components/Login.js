import * as React from 'react';
import Typography from "@mui/material/Typography";
import WarningAmberIcon from '@mui/icons-material/WarningAmber';
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import Box from "@mui/material/Box";
import api from "../app/Api";
import routes from "./routes";
import session from "../app/Session";
import {NavLink} from "react-router-dom";
import AvatarBox from "./AvatarBox";
import {useTranslation} from "react-i18next";
import {useState} from "react";

const Login = () => {
    const { t } = useTranslation();
    const [error, setError] = useState("");
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const handleSubmit = async (event) => {
        event.preventDefault();
        const user = { username, password };
        try {
            const token = await api.login(config.baseUrl, user);
            if (token) {
                console.log(`[Login] User auth for user ${user.username} successful, token is ${token}`);
                session.store(user.username, token);
                window.location.href = routes.app;
            } else {
                console.log(`[Login] User auth for user ${user.username} failed, access denied`);
                setError(t("Login failed: Invalid username or password"));
            }
        } catch (e) {
            console.log(`[Login] User auth for user ${user.username} failed`, e);
            if (e && e.message) {
                setError(e.message);
            } else {
                setError(t("Unknown error. Check logs for details."))
            }
        }
    };
    if (!config.enableLogin) {
        return (
            <AvatarBox>
                <Typography sx={{ typography: 'h6' }}>{t("Login is disabled")}</Typography>
            </AvatarBox>
        );
    }
    return (
        <AvatarBox>
            <Typography sx={{ typography: 'h6' }}>
                {t("Sign in to your ntfy account")}
            </Typography>
            <Box component="form" onSubmit={handleSubmit} noValidate sx={{mt: 1, maxWidth: 400}}>
                <TextField
                    margin="dense"
                    required
                    fullWidth
                    id="username"
                    label={t("Username")}
                    name="username"
                    value={username}
                    onChange={ev => setUsername(ev.target.value.trim())}
                    autoFocus
                />
                <TextField
                    margin="dense"
                    required
                    fullWidth
                    name="password"
                    label={t("Password")}
                    type="password"
                    id="password"
                    value={password}
                    onChange={ev => setPassword(ev.target.value.trim())}
                    autoComplete="current-password"
                />
                <Button
                    type="submit"
                    fullWidth
                    variant="contained"
                    disabled={username === "" || password === ""}
                    sx={{mt: 2, mb: 2}}
                >
                    {t("Sign in")}
                </Button>
                {error &&
                    <Box sx={{
                        mb: 1,
                        display: 'flex',
                        flexGrow: 1,
                        justifyContent: 'center',
                    }}>
                        <WarningAmberIcon color="error" sx={{mr: 1}}/>
                        <Typography sx={{color: 'error.main'}}>{error}</Typography>
                    </Box>
                }
                <Box sx={{width: "100%"}}>
                    {config.enableResetPassword && <div style={{float: "left"}}><NavLink to={routes.resetPassword} variant="body1">{t("Reset password")}</NavLink></div>}
                    {config.enableSignup && <div style={{float: "right"}}><NavLink to={routes.signup} variant="body1">{t("Sign up")}</NavLink></div>}
                </Box>
            </Box>
        </AvatarBox>
    );
}

export default Login;
