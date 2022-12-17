import * as React from 'react';
import {useState} from 'react';
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
import api from "../app/Api";
import routes from "./routes";
import IconButton from "@mui/material/IconButton";
import {NavLink, useOutletContext} from "react-router-dom";
import Box from "@mui/material/Box";

const Account = () => {
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
        <Card sx={{p: 3}} aria-label={t("xxxxxxxxx")}>
            <Typography variant="h5" sx={{marginBottom: 2}}>
                Account
            </Typography>
            <PrefGroup>
                <Username/>
                <ChangePassword/>
            </PrefGroup>
        </Card>
    );
};

const Stats = () => {
    const { t } = useTranslation();
    const { account } = useOutletContext();
    const admin = account?.role === "admin"
    const accountType = account?.plan?.name ?? "Free";
    return (
        <Card sx={{p: 3}} aria-label={t("xxxxxxxxx")}>
            <Typography variant="h5" sx={{marginBottom: 2}}>
                {t("Usage")}
            </Typography>
            <PrefGroup>
                <Pref labelId={"accountType"} title={t("Account type")}>
                    <div>
                        {account?.role === "admin"
                            ? <>Unlimited <Tooltip title={"You are Admin"}><span style={{cursor: "default"}}>👑</span></Tooltip></>
                            : accountType}
                    </div>
                </Pref>
                <Pref labelId={"dailyMessages"} title={t("Daily messages")}>
                    <div>
                        <Typography variant="body2" sx={{float: "left"}}>123</Typography>
                        <Typography variant="body2" sx={{float: "right"}}>of 1000</Typography>
                    </div>
                    <LinearProgress variant="determinate" value={10} />
                </Pref>
                <Pref labelId={"attachmentStorage"} title={t("Attachment storage")}>
                    <div>
                        <Typography variant="body2" sx={{float: "left"}}>15 MB used</Typography>
                        <Typography variant="body2" sx={{float: "right"}}>of 150 MB</Typography>
                    </div>
                    <LinearProgress variant="determinate" value={40} />
                </Pref>
                <Pref labelId={"emailLimits"} title={t("Emails sent")}>
                    <div>
                        <Typography variant="body2" sx={{float: "left"}}>2</Typography>
                        <Typography variant="body2" sx={{float: "right"}}>of 15</Typography>
                    </div>
                    <LinearProgress variant="determinate" value={20} />
                </Pref>
            </PrefGroup>
        </Card>
    );
};

const Delete = () => {
    const { t } = useTranslation();
    return (
        <Card sx={{p: 3}} aria-label={t("xxxxxxxxx")}>
            <Typography variant="h5" sx={{marginBottom: 2}}>
                {t("Delete account")}
            </Typography>
            <PrefGroup>
                <DeleteAccount/>
            </PrefGroup>
        </Card>
    );
};

const Username = () => {
    const { t } = useTranslation();
    const { account } = useOutletContext();
    return (
        <Pref labelId={"username"} title={t("Username")} description={t("Hey, that's you ❤")}>
            <div>
                {session.username()}
                {account?.role === "admin"
                    ? <>{" "}<Tooltip title={"You are Admin"}><span style={{cursor: "default"}}>👑</span></Tooltip></>
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
            await api.changePassword("http://localhost:2586", session.token(), newPassword);
            setDialogOpen(false);
            console.debug(`[Account] Password changed`);
        } catch (e) {
            console.log(`[Account] Error changing password`, e);
            // TODO show error
        }
    };
    return (
        <Pref labelId={labelId} title={t("Password")} description={t("Change your account password")}>
            <div>
                <Typography color="gray" sx={{float: "left", fontSize: "0.7rem", lineHeight: "3.5"}}>⬤⬤⬤⬤⬤⬤⬤⬤⬤⬤</Typography>
                <IconButton onClick={handleDialogOpen} aria-label={t("xxxxxxxx")}>
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
            <DialogTitle>Change password</DialogTitle>
            <DialogContent>
                <TextField
                    margin="dense"
                    id="new-password"
                    label={t("New password")}
                    aria-label={t("xxxx")}
                    type="password"
                    value={newPassword}
                    onChange={ev => setNewPassword(ev.target.value)}
                    fullWidth
                    variant="standard"
                />
                <TextField
                    margin="dense"
                    id="confirm"
                    label={t("Confirm password")}
                    aria-label={t("xxx")}
                    type="password"
                    value={confirmPassword}
                    onChange={ev => setConfirmPassword(ev.target.value)}
                    fullWidth
                    variant="standard"
                />
            </DialogContent>
            <DialogActions>
                <Button onClick={props.onCancel}>{t("Cancel")}</Button>
                <Button onClick={() => props.onSubmit(newPassword)} disabled={!changeButtonEnabled}>{t("Change password")}</Button>
            </DialogActions>
        </Dialog>
    );
};

const DeleteAccount = () => {
    const { t } = useTranslation();
    const [dialogKey, setDialogKey] = useState(0);
    const [dialogOpen, setDialogOpen] = useState(false);
    const labelId = "prefDeleteAccount";
    const handleDialogOpen = () => {
        setDialogKey(prev => prev+1);
        setDialogOpen(true);
    };
    const handleDialogCancel = () => {
        setDialogOpen(false);
    };
    const handleDialogSubmit = async (newPassword) => {
        try {
            await api.deleteAccount("http://localhost:2586", session.token());
            setDialogOpen(false);
            console.debug(`[Account] Account deleted`);
            // TODO delete local storage
            session.reset();
            window.location.href = routes.app;
        } catch (e) {
            console.log(`[Account] Error deleting account`, e);
            // TODO show error
        }
    };
    return (
        <Pref labelId={labelId} title={t("Delete account")} description={t("Permanently delete your account")}>
            <div>
                <Button fullWidth={false} variant="outlined" color="error" startIcon={<DeleteOutlineIcon />} onClick={handleDialogOpen}>
                    Delete account
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
            <DialogTitle>{t("Delete account")}</DialogTitle>
            <DialogContent>
                <Typography variant="body1">
                    {t("This will permanently delete your account, including all data that is stored on the server. If you really want to proceed, please type '{{username}}' in the text box below.", { username: session.username()})}
                </Typography>
                <TextField
                    margin="dense"
                    id="account-delete-confirm"
                    label={t("Type '{{username}}' to delete account", { username: session.username()})}
                    aria-label={t("xxxx")}
                    type="text"
                    value={username}
                    onChange={ev => setUsername(ev.target.value)}
                    fullWidth
                    variant="standard"
                />
            </DialogContent>
            <DialogActions>
                <Button onClick={props.onCancel}>{t("prefs_users_dialog_button_cancel")}</Button>
                <Button onClick={props.onSubmit} color="error" disabled={!buttonEnabled}>{t("Permanently delete account")}</Button>
            </DialogActions>
        </Dialog>
    );
};


// FIXME duplicate code

const PrefGroup = (props) => {
    return (
        <div role="table">
            {props.children}
        </div>
    )
};

const Pref = (props) => {
    return (
        <div
            role="row"
            style={{
                display: "flex",
                flexDirection: "row",
                marginTop: "10px",
                marginBottom: "20px",
            }}
        >
            <div
                role="cell"
                id={props.labelId}
                aria-label={props.title}
                style={{
                    flex: '1 0 40%',
                    display: 'flex',
                    flexDirection: 'column',
                    justifyContent: 'center',
                    paddingRight: '30px'
                }}
            >
                <div><b>{props.title}</b></div>
                {props.description && <div><em>{props.description}</em></div>}
            </div>
            <div
                role="cell"
                style={{
                    flex: '1 0 calc(60% - 50px)',
                    display: 'flex',
                    flexDirection: 'column',
                    justifyContent: 'center'
                }}
            >
                {props.children}
            </div>
        </div>
    );
};

export default Account;
