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
import {InputAdornment} from "@mui/material";
import IconButton from "@mui/material/IconButton";
import {Visibility, VisibilityOff} from "@mui/icons-material";

const Signup = () => {
    const { t } = useTranslation();
    const [error, setError] = useState("");
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const [confirm, setConfirm] = useState("");
    const [showPassword, setShowPassword] = useState(false);
    const [showConfirm, setShowConfirm] = useState(false);

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
                setError(t("signup_error_username_taken", { username: e.username }));
            } else if ((e instanceof AccountCreateLimitReachedError)) {
                setError(t("signup_error_creation_limit_reached"));
            } else if (e.message) {
                setError(e.message);
            } else {
                setError(t("signup_error_unknown"))
            }
        }
    };

    if (!config.enable_signup) {
        return (
            <AvatarBox>
                <Typography sx={{ typography: 'h6' }}>{t("signup_disabled")}</Typography>
            </AvatarBox>
        );
    }

    return (
        <AvatarBox>
            <Typography sx={{ typography: 'h6' }}>
                {t("signup_title")}
            </Typography>
            <Box component="form" onSubmit={handleSubmit} noValidate sx={{mt: 1, maxWidth: 400}}>
                <TextField
                    margin="dense"
                    required
                    fullWidth
                    id="username"
                    label={t("signup_form_username")}
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
                    label={t("signup_form_password")}
                    type={showPassword ? "text" : "password"}
                    id="password"
                    autoComplete="new-password"
                    value={password}
                    onChange={ev => setPassword(ev.target.value.trim())}
                    InputProps={{
                        endAdornment: (
                            <InputAdornment position="end">
                                <IconButton
                                    aria-label={t("signup_form_toggle_password_visibility")}
                                    onClick={() => setShowPassword(!showPassword)}
                                    onMouseDown={(ev) => ev.preventDefault()}
                                    edge="end"
                                >
                                    {showPassword ? <VisibilityOff /> : <Visibility />}
                                </IconButton>
                            </InputAdornment>
                        )
                    }}
                />
                <TextField
                    margin="dense"
                    required
                    fullWidth
                    name="password"
                    label={t("signup_form_confirm_password")}
                    type={showConfirm ? "text" : "password"}
                    id="confirm"
                    autoComplete="new-password"
                    value={confirm}
                    onChange={ev => setConfirm(ev.target.value.trim())}
                    InputProps={{
                        endAdornment: (
                            <InputAdornment position="end">
                                <IconButton
                                    aria-label={t("signup_form_toggle_password_visibility")}
                                    onClick={() => setShowConfirm(!showConfirm)}
                                    onMouseDown={(ev) => ev.preventDefault()}
                                    edge="end"
                                >
                                    {showConfirm ? <VisibilityOff /> : <Visibility />}
                                </IconButton>
                            </InputAdornment>
                        )
                    }}
                />
                <Button
                    type="submit"
                    fullWidth
                    variant="contained"
                    disabled={username === "" || password === "" || password !== confirm}
                    sx={{mt: 2, mb: 2}}
                >
                    {t("signup_form_button_submit")}
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
            {config.enable_login &&
                <Typography sx={{mb: 4}}>
                    <NavLink to={routes.login} variant="body1">
                        {t("signup_already_have_account")}
                    </NavLink>
                </Typography>
            }
        </AvatarBox>
    );
}

export default Signup;
