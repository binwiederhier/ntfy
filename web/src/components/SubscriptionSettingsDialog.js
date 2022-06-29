import * as React from 'react';
import {useState} from 'react';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import {Autocomplete, Checkbox, FormControlLabel, useMediaQuery} from "@mui/material";
import theme from "./theme";
import api from "../app/Api";
import {topicUrl, validTopic, validUrl} from "../app/utils";
import userManager from "../app/UserManager";
import subscriptionManager from "../app/SubscriptionManager";
import poller from "../app/Poller";
import DialogFooter from "./DialogFooter";
import {useTranslation} from "react-i18next";

const SubscriptionSettingsDialog = (props) => {
    const { t } = useTranslation();
    const subscription = props.subscription;
    const [displayName, setDisplayName] = useState(subscription.displayName ?? "");
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const handleSave = async () => {
        await subscriptionManager.setDisplayName(subscription.id, displayName);
        props.onClose();
    }
    return (
        <Dialog open={props.open} onClose={props.onClose} fullScreen={fullScreen}>
            <DialogTitle>{t("subscription_settings_dialog_title")}</DialogTitle>
            <DialogContent>
                <DialogContentText>
                    {t("subscription_settings_dialog_description")}
                </DialogContentText>
                <TextField
                    autoFocus
                    margin="dense"
                    id="topic"
                    placeholder={t("subscription_settings_dialog_display_name_placeholder")}
                    value={displayName}
                    onChange={ev => setDisplayName(ev.target.value)}
                    type="text"
                    fullWidth
                    variant="standard"
                    inputProps={{
                        maxLength: 64,
                        "aria-label": t("subscription_settings_dialog_display_name_placeholder")
                    }}
                />
            </DialogContent>
            <DialogFooter>
                <Button onClick={props.onClose}>{t("subscription_settings_button_cancel")}</Button>
                <Button onClick={handleSave}>{t("subscription_settings_button_save")}</Button>
            </DialogFooter>
        </Dialog>
    );
};

export default SubscriptionSettingsDialog;
