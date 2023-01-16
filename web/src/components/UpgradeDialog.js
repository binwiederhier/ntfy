import * as React from 'react';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import {Alert, CardActionArea, CardContent, useMediaQuery} from "@mui/material";
import theme from "./theme";
import DialogFooter from "./DialogFooter";
import Button from "@mui/material/Button";
import accountApi, {TopicReservedError, UnauthorizedError} from "../app/AccountApi";
import session from "../app/Session";
import routes from "./routes";
import {useContext, useState} from "react";
import Card from "@mui/material/Card";
import Typography from "@mui/material/Typography";
import {AccountContext} from "./App";
import {formatShortDate} from "../app/utils";
import {useTranslation} from "react-i18next";

const UpgradeDialog = (props) => {
    const { t } = useTranslation();
    const { account } = useContext(AccountContext);
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const [newTier, setNewTier] = useState(account?.tier?.code || null);
    const [errorText, setErrorText] = useState("");

    if (!account) {
        return <></>;
    }

    const currentTier = account.tier?.code || null;
    let action, submitButtonLabel, submitButtonEnabled;
    if (currentTier === newTier) {
        submitButtonLabel = "Update subscription";
        submitButtonEnabled = false;
        action = null;
    } else if (currentTier === null) {
        submitButtonLabel = "Pay $5 now and subscribe";
        submitButtonEnabled = true;
        action = Action.CREATE;
    } else if (newTier === null) {
        submitButtonLabel = "Cancel subscription";
        submitButtonEnabled = true;
        action = Action.CANCEL;
    } else {
        submitButtonLabel = "Update subscription";
        submitButtonEnabled = true;
        action = Action.UPDATE;
    }

    const handleSubmit = async () => {
        try {
            if (action === Action.CREATE) {
                const response = await accountApi.createBillingSubscription(newTier);
                window.location.href = response.redirect_url;
            } else if (action === Action.UPDATE) {
                await accountApi.updateBillingSubscription(newTier);
            } else if (action === Action.CANCEL) {
                await accountApi.deleteBillingSubscription();
            }
            props.onCancel();
        } catch (e) {
            console.log(`[UpgradeDialog] Error changing billing subscription`, e);
            if ((e instanceof UnauthorizedError)) {
                session.resetAndRedirect(routes.login);
            }
            // FIXME show error
        }
    }

    return (
        <Dialog open={props.open} onClose={props.onCancel} maxWidth="md" fullScreen={fullScreen}>
            <DialogTitle>Change billing plan</DialogTitle>
            <DialogContent>
                <div style={{
                    display: "flex",
                    flexDirection: "row"
                }}>
                    <TierCard code={null} name={"Free"} selected={newTier === null} onClick={() => setNewTier(null)}/>
                    <TierCard code="starter" name={"Starter"} selected={newTier === "starter"} onClick={() => setNewTier("starter")}/>
                    <TierCard code="pro" name={"Pro"} selected={newTier === "pro"} onClick={() => setNewTier("pro")}/>
                    <TierCard code="business" name={"Business"} selected={newTier === "business"} onClick={() => setNewTier("business")}/>
                </div>
                {action === Action.CANCEL &&
                    <Alert severity="warning">
                        {t("account_upgrade_dialog_cancel_warning", { date: formatShortDate(account.billing.paid_until) })}
                    </Alert>
                }
            </DialogContent>
            <DialogFooter status={errorText}>
                <Button onClick={props.onCancel}>Cancel</Button>
                <Button onClick={handleSubmit} disabled={!submitButtonEnabled}>{submitButtonLabel}</Button>
            </DialogFooter>
        </Dialog>
    );
};

const TierCard = (props) => {
    const cardStyle = (props.selected) ? {
        background: "#eee"
    } : {};
    return (
        <Card sx={{ m: 1, maxWidth: 345 }}>
            <CardActionArea>
                <CardContent sx={{...cardStyle}} onClick={props.onClick}>
                    <Typography gutterBottom variant="h5" component="div">
                        {props.name}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Lizards are a widespread group of squamate reptiles, with over 6,000
                        species, ranging across all continents except Antarctica
                    </Typography>
                </CardContent>
            </CardActionArea>
        </Card>
    );
}

const Action = {
    CREATE: 1,
    UPDATE: 2,
    CANCEL: 3
};

export default UpgradeDialog;
