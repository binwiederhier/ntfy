import * as React from 'react';
import {useState} from 'react';
import TextField from "@mui/material/TextField";
import Button from "@mui/material/Button";
import Box from "@mui/material/Box";
import routes from "./routes";
import session from "../app/Session";
import Typography from "@mui/material/Typography";
import {NavLink} from "react-router-dom";
import AvatarBox from "./AvatarBox";
import {useTranslation} from "react-i18next";
import WarningAmberIcon from "@mui/icons-material/WarningAmber";
import accountApi, {AccountCreateLimitReachedError, UsernameTakenError} from "../app/AccountApi";

const Signup = () => {
    const { t } = useTranslation();
    const [error, setError] = useState("");
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const [confirm, setConfirm] = useState("");
    const handleSubmit = async (event) => {
        event.preventDefault();
        const user = { username, password };
        try {
            await accountApi.create(user.username, user.password);
            const token = await accountApi.login(user);
            console.log(`[Signup] User signup for user ${user.username} successful, token is ${token}`);
            session.store(user.username, token);
            window.location.href = routes.app;
        } catch (e) {
            console.log(`[Signup] Signup for user ${user.username} failed`, e);
            if ((e instanceof UsernameTakenError)) {
                setError(t("Username {{username}} is already taken", { username: e.username }));
            } else if ((e instanceof AccountCreateLimitReachedError)) {
                setError(t("Account creation limit reached"));
            } else if (e.message) {
                setError(e.message);
            } else {
                setError(t("Unknown error. Check logs for details."))
            }
        }
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
                    value={username}
                    onChange={ev => setUsername(ev.target.value.trim())}
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
                    value={password}
                    onChange={ev => setPassword(ev.target.value.trim())}
                />
                <TextField
                    margin="dense"
                    required
                    fullWidth
                    name="confirm-password"
                    label="Confirm password"
                    type="password"
                    id="confirm-password"
                    value={confirm}
                    onChange={ev => setConfirm(ev.target.value.trim())}

                />
                <Button
                    type="submit"
                    fullWidth
                    variant="contained"
                    disabled={username === "" || password === "" || password !== confirm}
                    sx={{mt: 2, mb: 2}}
                >
                    {t("Sign up")}
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
