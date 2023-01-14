import * as React from 'react';
import {useContext, useState} from 'react';
import {LinearProgress, Stack, useMediaQuery} from "@mui/material";
import Tooltip from '@mui/material/Tooltip';
import Typography from "@mui/material/Typography";
import EditIcon from '@mui/icons-material/Edit';
import Container from "@mui/material/Container";
import Card from "@mui/material/Card";
import Button from "@mui/material/Button";
import {useTranslation} from "react-i18next";
import session from "../app/Session";
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import theme from "./theme";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import TextField from "@mui/material/TextField";
import DialogActions from "@mui/material/DialogActions";
import routes from "./routes";
import IconButton from "@mui/material/IconButton";
import {formatBytes} from "../app/utils";
import accountApi, {UnauthorizedError} from "../app/AccountApi";
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import {Pref, PrefGroup} from "./Pref";
import db from "../app/db";
import i18n from "i18next";
import humanizeDuration from "humanize-duration";
import UpgradeDialog from "./UpgradeDialog";
import CelebrationIcon from "@mui/icons-material/Celebration";
import {AccountContext} from "./App";

const Account = () => {
    if (!session.exists()) {
        window.location.href = routes.app;
        return <></>;
    }
    return (
        <Container maxWidth="md" sx={{marginTop: 3, marginBottom: 3}}>
            <Stack spacing={3}>
                <Basics/>
                <Stats/>
                <Delete/>
            </Stack>
        </Container>
    );
};

const Basics = () => {
    const { t } = useTranslation();
    return (
        <Card sx={{p: 3}} aria-label={t("account_basics_title")}>
            <Typography variant="h5" sx={{marginBottom: 2}}>
                {t("account_basics_title")}
            </Typography>
            <PrefGroup>
                <Username/>
                <ChangePassword/>
            </PrefGroup>
        </Card>
    );
};

const Username = () => {
    const { t } = useTranslation();
    const { account } = useContext(AccountContext);
    const labelId = "prefUsername";

    return (
        <Pref labelId={labelId} title={t("account_basics_username_title")} description={t("account_basics_username_description")}>
            <div aria-labelledby={labelId}>
                {session.username()}
                {account?.role === "admin"
                    ? <>{" "}<Tooltip title={t("account_basics_username_admin_tooltip")}><span style={{cursor: "default"}}>👑</span></Tooltip></>
                    : ""}
            </div>
        </Pref>
    )
};

const ChangePassword = () => {
    const { t } = useTranslation();
    const [dialogKey, setDialogKey] = useState(0);
    const [dialogOpen, setDialogOpen] = useState(false);
    const labelId = "prefChangePassword";

    const handleDialogOpen = () => {
        setDialogKey(prev => prev+1);
        setDialogOpen(true);
    };

    const handleDialogCancel = () => {
        setDialogOpen(false);
    };

    const handleDialogSubmit = async (newPassword) => {
        try {
            await accountApi.changePassword(newPassword);
            setDialogOpen(false);
            console.debug(`[Account] Password changed`);
        } catch (e) {
            console.log(`[Account] Error changing password`, e);
            if ((e instanceof UnauthorizedError)) {
                session.resetAndRedirect(routes.login);
            }
            // TODO show error
        }
    };

    return (
        <Pref labelId={labelId} title={t("account_basics_password_title")} description={t("account_basics_password_description")}>
            <div aria-labelledby={labelId}>
                <Typography color="gray" sx={{float: "left", fontSize: "0.7rem", lineHeight: "3.5"}}>⬤⬤⬤⬤⬤⬤⬤⬤⬤⬤</Typography>
                <IconButton onClick={handleDialogOpen} aria-label={t("account_basics_password_description")}>
                    <EditIcon/>
                </IconButton>
            </div>
            <ChangePasswordDialog
                key={`changePasswordDialog${dialogKey}`}
                open={dialogOpen}
                onCancel={handleDialogCancel}
                onSubmit={handleDialogSubmit}
            />
        </Pref>
    )
};

const ChangePasswordDialog = (props) => {
    const { t } = useTranslation();
    const [newPassword, setNewPassword] = useState("");
    const [confirmPassword, setConfirmPassword] = useState("");
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const changeButtonEnabled = (() => {
        return newPassword.length > 0 && newPassword === confirmPassword;
    })();
    return (
        <Dialog open={props.open} onClose={props.onCancel} fullScreen={fullScreen}>
            <DialogTitle>{t("account_basics_password_dialog_title")}</DialogTitle>
            <DialogContent>
                <TextField
                    margin="dense"
                    id="new-password"
                    label={t("account_basics_password_dialog_new_password_label")}
                    aria-label={t("account_basics_password_dialog_new_password_label")}
                    type="password"
                    value={newPassword}
                    onChange={ev => setNewPassword(ev.target.value)}
                    fullWidth
                    variant="standard"
                />
                <TextField
                    margin="dense"
                    id="confirm"
                    label={t("account_basics_password_dialog_confirm_password_label")}
                    aria-label={t("account_basics_password_dialog_confirm_password_label")}
                    type="password"
                    value={confirmPassword}
                    onChange={ev => setConfirmPassword(ev.target.value)}
                    fullWidth
                    variant="standard"
                />
            </DialogContent>
            <DialogActions>
                <Button onClick={props.onCancel}>{t("account_basics_password_dialog_button_cancel")}</Button>
                <Button onClick={() => props.onSubmit(newPassword)} disabled={!changeButtonEnabled}>{t("account_basics_password_dialog_button_submit")}</Button>
            </DialogActions>
        </Dialog>
    );
};

const Stats = () => {
    const { t } = useTranslation();
    const { account } = useContext(AccountContext);
    const [upgradeDialogOpen, setUpgradeDialogOpen] = useState(false);

    if (!account) {
        return <></>;
    }

    const normalize = (value, max) => {
        return Math.min(value / max * 100, 100);
    };

    const handleManageBilling = async () => {
        try {
            const response = await accountApi.createBillingPortalSession();
            window.location.href = response.redirect_url;
        } catch (e) {
            console.log(`[Account] Error changing password`, e);
            if ((e instanceof UnauthorizedError)) {
                session.resetAndRedirect(routes.login);
            }
            // TODO show error
        }
    };

    return (
        <Card sx={{p: 3}} aria-label={t("account_usage_title")}>
            <Typography variant="h5" sx={{marginBottom: 2}}>
                {t("account_usage_title")}
            </Typography>
            <PrefGroup>
                <Pref title={t("account_usage_tier_title")}>
                    <div>
                        {account.role === "admin" &&
                            <>
                                {t("account_usage_tier_admin")}
                                {" "}{account.tier ? `(with ${account.tier.name} tier)` : `(no tier)`}
                            </>
                        }
                        {account.role === "user" && account.tier && account.tier.name}
                        {account.role === "user" && !account.tier && t("account_usage_tier_none")}
                        {config.enable_payments && account.role === "user" && (!account.tier || !account.tier.paid) &&
                            <Button
                                variant="outlined"
                                size="small"
                                startIcon={<CelebrationIcon sx={{ color: "#55b86e" }}/>}
                                onClick={() => setUpgradeDialogOpen(true)}
                                sx={{ml: 1}}
                            >{t("account_usage_tier_upgrade_button")}</Button>
                        }
                        {config.enable_payments && account.role === "user" && account.tier?.paid &&
                            <>
                                <Button
                                    variant="outlined"
                                    size="small"
                                    onClick={() => setUpgradeDialogOpen(true)}
                                    sx={{ml: 1}}
                                >{t("account_usage_tier_change_button")}</Button>
                                <Button
                                    variant="outlined"
                                    size="small"
                                    onClick={handleManageBilling}
                                    sx={{ml: 1}}
                                >Manage billing</Button>
                            </>
                        }
                        <UpgradeDialog
                            open={upgradeDialogOpen}
                            onCancel={() => setUpgradeDialogOpen(false)}
                        />
                    </div>
                </Pref>
                {account.role !== "admin" &&
                    <Pref title={t("account_usage_reservations_title")}>
                        {account.limits.reservations > 0 &&
                            <>
                                <div>
                                    <Typography variant="body2"
                                                sx={{float: "left"}}>{account.stats.reservations}</Typography>
                                    <Typography variant="body2"
                                                sx={{float: "right"}}>{account.role === "user" ? t("account_usage_of_limit", {limit: account.limits.reservations}) : t("account_usage_unlimited")}</Typography>
                                </div>
                                <LinearProgress
                                    variant="determinate"
                                    value={account.limits.reservations > 0 ? normalize(account.stats.reservations, account.limits.reservations) : 100}
                                />
                            </>
                        }
                        {account.limits.reservations === 0 &&
                            <em>No reserved topics for this account</em>
                        }
                    </Pref>
                }
                <Pref title={
                    <>
                        {t("account_usage_messages_title")}
                        <Tooltip title={t("account_usage_limits_reset_daily")}><span><InfoIcon/></span></Tooltip>
                    </>
                }>
                    <div>
                        <Typography variant="body2" sx={{float: "left"}}>{account.stats.messages}</Typography>
                        <Typography variant="body2" sx={{float: "right"}}>{account.role === "user" ? t("account_usage_of_limit", { limit: account.limits.messages }) : t("account_usage_unlimited")}</Typography>
                    </div>
                    <LinearProgress
                        variant="determinate"
                        value={account.role === "user" ? normalize(account.stats.messages, account.limits.messages) : 100}
                    />
                </Pref>
                <Pref title={
                    <>
                        {t("account_usage_emails_title")}
                        <Tooltip title={t("account_usage_limits_reset_daily")}><span><InfoIcon/></span></Tooltip>
                    </>
                }>
                    <div>
                        <Typography variant="body2" sx={{float: "left"}}>{account.stats.emails}</Typography>
                        <Typography variant="body2" sx={{float: "right"}}>{account.role === "user" ? t("account_usage_of_limit", { limit: account.limits.emails }) : t("account_usage_unlimited")}</Typography>
                    </div>
                    <LinearProgress
                        variant="determinate"
                        value={account.role === "user" ? normalize(account.stats.emails, account.limits.emails) : 100}
                    />
                </Pref>
                <Pref
                    alignTop
                    title={t("account_usage_attachment_storage_title")}
                    description={t("account_usage_attachment_storage_description", {
                        filesize: formatBytes(account.limits.attachment_file_size),
                        expiry: humanizeDuration(account.limits.attachment_expiry_duration * 1000, {
                            language: i18n.language,
                            fallbacks: ["en"]
                        })
                    })}
                >
                    <div>
                        <Typography variant="body2" sx={{float: "left"}}>{formatBytes(account.stats.attachment_total_size)}</Typography>
                        <Typography variant="body2" sx={{float: "right"}}>{account.role === "user" ? t("account_usage_of_limit", { limit: formatBytes(account.limits.attachment_total_size) }) : t("account_usage_unlimited")}</Typography>
                    </div>
                    <LinearProgress
                        variant="determinate"
                        value={account.role === "user" ? normalize(account.stats.attachment_total_size, account.limits.attachment_total_size) : 100}
                    />
                </Pref>
            </PrefGroup>
            {account.role === "user" && account.limits.basis === "ip" &&
                <Typography variant="body1">
                    {t("account_usage_basis_ip_description")}
                </Typography>
            }
        </Card>
    );
};

const InfoIcon = () => {
    return (
        <InfoOutlinedIcon sx={{
            verticalAlign: "bottom",
            width: "18px",
            marginLeft: "4px",
            color: "gray"
        }}/>
    );
}

const Delete = () => {
    const { t } = useTranslation();
    return (
        <Card sx={{p: 3}} aria-label={t("account_delete_title")}>
            <Typography variant="h5" sx={{marginBottom: 2}}>
                {t("account_delete_title")}
            </Typography>
            <PrefGroup>
                <DeleteAccount/>
            </PrefGroup>
        </Card>
    );
};

const DeleteAccount = () => {
    const { t } = useTranslation();
    const [dialogKey, setDialogKey] = useState(0);
    const [dialogOpen, setDialogOpen] = useState(false);

    const handleDialogOpen = () => {
        setDialogKey(prev => prev+1);
        setDialogOpen(true);
    };

    const handleDialogCancel = () => {
        setDialogOpen(false);
    };

    const handleDialogSubmit = async () => {
        try {
            await accountApi.delete();
            await db.delete();
            setDialogOpen(false);
            console.debug(`[Account] Account deleted`);
            session.resetAndRedirect(routes.app);
        } catch (e) {
            console.log(`[Account] Error deleting account`, e);
            if ((e instanceof UnauthorizedError)) {
                session.resetAndRedirect(routes.login);
            }
            // TODO show error
        }
    };

    return (
        <Pref title={t("account_delete_title")} description={t("account_delete_description")}>
            <div>
                <Button fullWidth={false} variant="outlined" color="error" startIcon={<DeleteOutlineIcon />} onClick={handleDialogOpen}>
                    {t("account_delete_title")}
                </Button>
            </div>
            <DeleteAccountDialog
                key={`deleteAccountDialog${dialogKey}`}
                open={dialogOpen}
                onCancel={handleDialogCancel}
                onSubmit={handleDialogSubmit}
            />
        </Pref>
    )
};

const DeleteAccountDialog = (props) => {
    const { t } = useTranslation();
    const [username, setUsername] = useState("");
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const buttonEnabled = username === session.username();
    return (
        <Dialog open={props.open} onClose={props.onCancel} fullScreen={fullScreen}>
            <DialogTitle>{t("account_delete_title")}</DialogTitle>
            <DialogContent>
                <Typography variant="body1">
                    {t("account_delete_dialog_description", { username: session.username()})}
                </Typography>
                <TextField
                    margin="dense"
                    id="account-delete-confirm"
                    label={t("account_delete_dialog_label", { username: session.username()})}
                    aria-label={t("account_delete_dialog_label", { username: session.username()})}
                    type="text"
                    value={username}
                    onChange={ev => setUsername(ev.target.value)}
                    fullWidth
                    variant="standard"
                />
            </DialogContent>
            <DialogActions>
                <Button onClick={props.onCancel}>{t("account_delete_dialog_button_cancel")}</Button>
                <Button onClick={props.onSubmit} color="error" disabled={!buttonEnabled}>{t("account_delete_dialog_button_submit")}</Button>
            </DialogActions>
        </Dialog>
    );
};

export default Account;
