import * as React from 'react';
import {useContext, useEffect, useState} from 'react';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import {
    Alert,
    Autocomplete,
    Checkbox,
    FormControl,
    FormControlLabel,
    FormGroup,
    Select,
    useMediaQuery
} from "@mui/material";
import theme from "./theme";
import api from "../app/Api";
import {randomAlphanumericString, topicUrl, validTopic, validUrl} from "../app/utils";
import userManager from "../app/UserManager";
import subscriptionManager from "../app/SubscriptionManager";
import poller from "../app/Poller";
import DialogFooter from "./DialogFooter";
import {useTranslation} from "react-i18next";
import session from "../app/Session";
import routes from "./routes";
import accountApi, {Permission, Role, TopicReservedError, UnauthorizedError} from "../app/AccountApi";
import ReserveTopicSelect from "./ReserveTopicSelect";
import {AccountContext} from "./App";
import DialogActions from "@mui/material/DialogActions";
import MenuItem from "@mui/material/MenuItem";
import ListItemIcon from "@mui/material/ListItemIcon";
import {PermissionDenyAll, PermissionRead, PermissionReadWrite, PermissionWrite} from "./ReserveIcons";
import ListItemText from "@mui/material/ListItemText";
import {Check, DeleteForever} from "@mui/icons-material";

export const ReserveAddDialog = (props) => {
    const { t } = useTranslation();
    const [topic, setTopic] = useState(props.topic || "");
    const [everyone, setEveryone] = useState(Permission.DENY_ALL);
    const [errorText, setErrorText] = useState("");
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const allowTopicEdit = !props.topic;
    const alreadyReserved = props.reservations.filter(r => r.topic === topic).length > 0;
    const submitButtonEnabled = validTopic(topic) && !alreadyReserved;

    const handleSubmit = async () => {
        try {
            await accountApi.upsertReservation(topic, everyone);
            console.debug(`[ReserveAddDialog] Added reservation for topic ${t}: ${everyone}`);
        } catch (e) {
            console.log(`[ReserveAddDialog] Error adding topic reservation.`, e);
            if ((e instanceof UnauthorizedError)) {
                session.resetAndRedirect(routes.login);
            } else if ((e instanceof TopicReservedError)) {
                setErrorText(t("subscribe_dialog_error_topic_already_reserved"));
                return;
            }
        }
        props.onClose();
        // FIXME handle 401/403/409
    };

    return (
        <Dialog open={props.open} onClose={props.onClose} maxWidth="sm" fullWidth fullScreen={fullScreen}>
            <DialogTitle>{t("prefs_reservations_dialog_title_add")}</DialogTitle>
            <DialogContent>
                <DialogContentText>
                    {t("prefs_reservations_dialog_description")}
                </DialogContentText>
                {allowTopicEdit && <TextField
                    autoFocus
                    margin="dense"
                    id="topic"
                    label={t("prefs_reservations_dialog_topic_label")}
                    aria-label={t("prefs_reservations_dialog_topic_label")}
                    value={topic}
                    onChange={ev => setTopic(ev.target.value)}
                    type="url"
                    fullWidth
                    variant="standard"
                />}
                <ReserveTopicSelect
                    value={everyone}
                    onChange={setEveryone}
                    sx={{mt: 1}}
                />
            </DialogContent>
            <DialogFooter status={errorText}>
                <Button onClick={props.onClose}>{t("prefs_users_dialog_button_cancel")}</Button>
                <Button onClick={handleSubmit} disabled={!submitButtonEnabled}>{t("prefs_users_dialog_button_add")}</Button>
            </DialogFooter>
        </Dialog>
    );
};

export const ReserveEditDialog = (props) => {
    const { t } = useTranslation();
    const [everyone, setEveryone] = useState(props.reservation?.everyone || Permission.DENY_ALL);
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));

    const handleSubmit = async () => {
        try {
            await accountApi.upsertReservation(props.reservation.topic, everyone);
            console.debug(`[ReserveEditDialog] Updated reservation for topic ${t}: ${everyone}`);
        } catch (e) {
            console.log(`[ReserveEditDialog] Error updating topic reservation.`, e);
            if ((e instanceof UnauthorizedError)) {
                session.resetAndRedirect(routes.login);
            }
        }
        props.onClose();
        // FIXME handle 401/403/409
    };

    return (
        <Dialog open={props.open} onClose={props.onClose} maxWidth="sm" fullWidth fullScreen={fullScreen}>
            <DialogTitle>{t("prefs_reservations_dialog_title_edit")}</DialogTitle>
            <DialogContent>
                <DialogContentText>
                    {t("prefs_reservations_dialog_description")}
                </DialogContentText>
                <ReserveTopicSelect
                    value={everyone}
                    onChange={setEveryone}
                    sx={{mt: 1}}
                />
            </DialogContent>
            <DialogActions>
                <Button onClick={props.onClose}>{t("common_cancel")}</Button>
                <Button onClick={handleSubmit}>{t("common_save")}</Button>
            </DialogActions>
        </Dialog>
    );
};

export const ReserveDeleteDialog = (props) => {
    const { t } = useTranslation();
    const [deleteMessages, setDeleteMessages] = useState(false);
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));

    const handleSubmit = async () => {
        try {
            await accountApi.deleteReservation(props.topic, deleteMessages);
            console.debug(`[ReserveDeleteDialog] Deleted reservation for topic ${t}`);
        } catch (e) {
            console.log(`[ReserveDeleteDialog] Error deleting topic reservation.`, e);
            if ((e instanceof UnauthorizedError)) {
                session.resetAndRedirect(routes.login);
            }
        }
        props.onClose();
        // FIXME handle 401/403/409
    };

    return (
        <Dialog open={props.open} onClose={props.onClose} maxWidth="sm" fullWidth fullScreen={fullScreen}>
            <DialogTitle>{t("prefs_reservations_dialog_title_delete")}</DialogTitle>
            <DialogContent>
                <DialogContentText>
                    {t("reservation_delete_dialog_description")}
                </DialogContentText>
                <FormControl fullWidth variant="standard">
                    <Select
                        value={deleteMessages}
                        onChange={(ev) => setDeleteMessages(ev.target.value)}
                        sx={{
                            "& .MuiSelect-select": {
                                display: 'flex',
                                alignItems: 'center',
                                paddingTop: "4px",
                                paddingBottom: "4px",
                            }
                        }}
                    >
                        <MenuItem value={false}>
                            <ListItemIcon><Check/></ListItemIcon>
                            <ListItemText primary={t("reservation_delete_dialog_action_keep_title")}/>
                        </MenuItem>
                        <MenuItem value={true}>
                            <ListItemIcon><DeleteForever/></ListItemIcon>
                            <ListItemText primary={t("reservation_delete_dialog_action_delete_title")}/>
                        </MenuItem>
                    </Select>
                </FormControl>
                {!deleteMessages &&
                    <Alert severity="info" sx={{ mt: 1 }}>
                        {t("reservation_delete_dialog_action_keep_description")}
                    </Alert>
                }
                {deleteMessages &&
                    <Alert severity="warning" sx={{ mt: 1 }}>
                        {t("reservation_delete_dialog_action_delete_description")}
                    </Alert>
                }
            </DialogContent>
            <DialogActions>
                <Button onClick={props.onClose}>{t("common_cancel")}</Button>
                <Button onClick={handleSubmit} color="error">{t("reservation_delete_dialog_submit_button")}</Button>
            </DialogActions>
        </Dialog>
    );
};

