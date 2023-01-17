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
import {useContext, useEffect, useState} from "react";
import Card from "@mui/material/Card";
import Typography from "@mui/material/Typography";
import {AccountContext} from "./App";
import {formatShortDate} from "../app/utils";
import {useTranslation} from "react-i18next";
import subscriptionManager from "../app/SubscriptionManager";

const UpgradeDialog = (props) => {
    const { t } = useTranslation();
    const { account } = useContext(AccountContext);
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const [tiers, setTiers] = useState(null);
    const [newTier, setNewTier] = useState(account?.tier?.code || null);
    const [errorText, setErrorText] = useState("");

    useEffect(() => {
        (async () => {
            setTiers(await accountApi.billingTiers());
        })();
    }, []);

    if (!account || !tiers) {
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
            <DialogTitle>{t("account_upgrade_dialog_title")}</DialogTitle>
            <DialogContent>
                <div style={{
                    display: "flex",
                    flexDirection: "row",
                    marginBottom: "8px",
                    width: "100%"
                }}>
                    <TierCard
                        code={null}
                        name={t("account_usage_tier_free")}
                        price={null}
                        selected={newTier === null}
                        onClick={() => setNewTier(null)}
                    />
                    {tiers.map(tier =>
                        <TierCard
                            key={`tierCard${tier.code}`}
                            code={tier.code}
                            name={tier.name}
                            price={tier.price}
                            features={tier.features}
                            selected={newTier === tier.code}
                            onClick={() => setNewTier(tier.code)}
                        />
                    )}
                </div>
                {action === Action.CANCEL &&
                    <Alert severity="warning">
                        {t("account_upgrade_dialog_cancel_warning", { date: formatShortDate(account.billing.paid_until) })}
                    </Alert>
                }
                {action === Action.UPDATE &&
                    <Alert severity="info">
                        {t("account_upgrade_dialog_proration_info")}
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
    const cardStyle = (props.selected) ? { background: "#eee", border: "2px solid #338574" } : {};
    return (
        <Card sx={{
            m: 1,
            minWidth: "190px",
            maxWidth: "250px",
            "&:first-child": { ml: 0 },
            "&:last-child": { mr: 0 },
            ...cardStyle
        }}>
            <CardActionArea sx={{ height: "100%" }}>
                <CardContent onClick={props.onClick} sx={{ height: "100%" }}>
                    {props.selected &&
                        <div style={{
                            position: "absolute",
                            top: "0",
                            right: "15px",
                            padding: "2px 10px",
                            background: "#338574",
                            color: "white",
                            borderRadius: "3px",
                        }}>Selected</div>
                    }
                    <Typography gutterBottom variant="h5" component="div">
                        {props.name}
                    </Typography>
                    {props.features &&
                        <Typography variant="body2" color="text.secondary" sx={{whiteSpace: "pre-wrap"}}>
                            {props.features}
                        </Typography>
                    }
                    {props.price &&
                        <Typography variant="subtitle1" sx={{mt: 1}}>
                            {props.price} / month
                        </Typography>
                    }
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
