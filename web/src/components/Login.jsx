import * as React from "react";
import { useState } from "react";
import { Typography, TextField, Button, Box, IconButton, InputAdornment } from "@mui/material";
import WarningAmberIcon from "@mui/icons-material/WarningAmber";
import { NavLink } from "react-router-dom";
import { useTranslation } from "react-i18next";
import Visibility from "@mui/icons-material/Visibility";
import VisibilityOff from "@mui/icons-material/VisibilityOff";
import accountApi from "../app/AccountApi";
import AvatarBox from "./AvatarBox";
import session from "../app/Session";
import routes from "./routes";
import { getShareFragment } from "../app/share";
import { UnauthorizedError } from "../app/errors";
import { fadeReload } from "../app/transition";

const Login = () => {
  const { t } = useTranslation();
  const [error, setError] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);

  const handleSubmit = async (event) => {
    event.preventDefault();
    const user = { username, password };
    try {
      const { token, username: canonicalUsername } = await accountApi.login(user);
      console.log(`[Login] User auth for user ${user.username} successful, logged in as ${canonicalUsername}`);
      await session.store(canonicalUsername, token);
      fadeReload(routes.app + getShareFragment(window.location.hash));
    } catch (e) {
      console.log(`[Login] User auth for user ${user.username} failed`, e);
      if (e instanceof UnauthorizedError) {
        setError(t("Login failed: Invalid username/email or password"));
      } else {
        setError(e.message);
      }
    }
  };
  if (!config.enable_login) {
    return (
      <AvatarBox>
        <Typography sx={{ typography: "h6" }}>{t("login_disabled")}</Typography>
      </AvatarBox>
    );
  }
  return (
    <AvatarBox>
      <Typography sx={{ typography: "h6" }}>{t("login_title")}</Typography>
      <Box component="form" onSubmit={handleSubmit} noValidate sx={{ mt: 1 }}>
        <TextField
          margin="dense"
          required
          fullWidth
          id="username"
          label={t("login_form_username_label")}
          name="username"
          value={username}
          onChange={(ev) => setUsername(ev.target.value.trim())}
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
          value={password}
          onChange={(ev) => setPassword(ev.target.value)}
          autoComplete="current-password"
          slotProps={{
            input: {
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
              ),
            },
          }}
        />
        <Button type="submit" fullWidth variant="contained" disabled={username === "" || password === ""} sx={{ mt: 2, mb: 2 }}>
          {t("login_form_button_submit")}
        </Button>
        {error && (
          <Box
            sx={{
              mb: 1,
              display: "flex",
              flexGrow: 1,
              justifyContent: "center",
            }}
          >
            <WarningAmberIcon color="error" sx={{ mr: 1 }} />
            <Typography sx={{ color: "error.main" }}>{error}</Typography>
          </Box>
        )}
        <Box sx={{ width: "100%" }}>
          {config.enable_reset_password && (
            <div style={{ float: "left" }}>
              <NavLink to={routes.passwordResetRequest} variant="body1">
                {t("login_link_forgot_password")}
              </NavLink>
            </div>
          )}
          {config.enable_signup && (
            <div style={{ float: "right" }}>
              <NavLink to={routes.signup} variant="body1">
                {t("login_link_signup")}
              </NavLink>
            </div>
          )}
        </Box>
      </Box>
    </AvatarBox>
  );
};

export default Login;
