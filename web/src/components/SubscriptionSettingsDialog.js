import * as React from 'react';
import {useState} from 'react';
import Button from '@mui/material/Button';
import TextField from '@mui/material/TextField';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import {Checkbox, FormControlLabel, useMediaQuery} from "@mui/material";
import theme from "./theme";
import subscriptionManager from "../app/SubscriptionManager";
import DialogFooter from "./DialogFooter";
import {useTranslation} from "react-i18next";
import accountApi, {UnauthorizedError} from "../app/AccountApi";
import session from "../app/Session";
import routes from "./routes";
import ReserveTopicSelect from "./ReserveTopicSelect";

const SubscriptionSettingsDialog = (props) => {
    const { t } = useTranslation();
    const subscription = props.subscription;
    const [reserveTopicVisible, setReserveTopicVisible] = useState(!!subscription.reservation);
    const [everyone, setEveryone] = useState(subscription.reservation?.everyone || "deny-all");
    const [displayName, setDisplayName] = useState(subscription.displayName ?? "");
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));

    const handleSave = async () => {
        // Apply locally
        await subscriptionManager.setDisplayName(subscription.id, displayName);

        // Apply remotely
        if (session.exists() && subscription.remoteId) {
            try {
                // Display name
                console.log(`[SubscriptionSettingsDialog] Updating subscription display name to ${displayName}`);
                await accountApi.updateSubscription(subscription.remoteId, { display_name: displayName });

                // Reservation
                if (reserveTopicVisible) {
                    await accountApi.upsertReservation(subscription.topic, everyone);
                } else if (!reserveTopicVisible && subscription.reservation) { // Was removed
                    await accountApi.deleteReservation(subscription.topic);
                }

                // Sync account
                await accountApi.sync();
            } catch (e) {
                console.log(`[SubscriptionSettingsDialog] Error updating subscription`, e);
                if ((e instanceof UnauthorizedError)) {
                    session.resetAndRedirect(routes.login);
                }

                // FIXME handle 409
            }
        }
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
                {config.enable_reservations && session.exists() &&
                    <>
                        <FormControlLabel
                            fullWidth
                            variant="standard"
                            sx={{pt: 1}}
                            control={
                                <Checkbox
                                    checked={reserveTopicVisible}
                                    onChange={(ev) => setReserveTopicVisible(ev.target.checked)}
                                    inputProps={{
                                        "aria-label": t("subscription_settings_dialog_reserve_topic_label")
                                    }}
                                />
                            }
                            label={t("subscription_settings_dialog_reserve_topic_label")}
                        />
                        {reserveTopicVisible &&
                            <ReserveTopicSelect
                                value={everyone}
                                onChange={setEveryone}
                            />
                        }
                    </>
                }
            </DialogContent>
            <DialogFooter>
                <Button onClick={props.onClose}>{t("subscription_settings_button_cancel")}</Button>
                <Button onClick={handleSave}>{t("subscription_settings_button_save")}</Button>
            </DialogFooter>
        </Dialog>
    );
};

export default SubscriptionSettingsDialog;
