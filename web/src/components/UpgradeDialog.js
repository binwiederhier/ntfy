import * as React from 'react';
import {useContext, useEffect, useState} from 'react';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import {Alert, CardActionArea, CardContent, Chip, Link, ListItem, Switch, useMediaQuery} from "@mui/material";
import theme from "./theme";
import Button from "@mui/material/Button";
import accountApi, {SubscriptionInterval} from "../app/AccountApi";
import session from "../app/Session";
import routes from "./routes";
import Card from "@mui/material/Card";
import Typography from "@mui/material/Typography";
import {AccountContext} from "./App";
import {formatBytes, formatNumber, formatPrice, formatShortDate} from "../app/utils";
import {Trans, useTranslation} from "react-i18next";
import List from "@mui/material/List";
import {Check, Close} from "@mui/icons-material";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText from "@mui/material/ListItemText";
import Box from "@mui/material/Box";
import {NavLink} from "react-router-dom";
import {UnauthorizedError} from "../app/errors";
import DialogContentText from "@mui/material/DialogContentText";
import DialogActions from "@mui/material/DialogActions";

const UpgradeDialog = (props) => {
    const { t } = useTranslation();
    const { account } = useContext(AccountContext); // May be undefined!
    const [error, setError] = useState("");
    const [tiers, setTiers] = useState(null);
    const [interval, setInterval] = useState(account?.billing?.interval || SubscriptionInterval.YEAR);
    const [newTierCode, setNewTierCode] = useState(account?.tier?.code); // May be undefined
    const [loading, setLoading] = useState(false);
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));

    useEffect(() => {
        const fetchTiers = async () => {
            setTiers(await accountApi.billingTiers());
        }
        fetchTiers(); // Dangle
    }, []);

    if (!tiers) {
        return <></>;
    }

    const tiersMap = Object.assign(...tiers.map(tier => ({[tier.code]: tier})));
    const newTier = tiersMap[newTierCode]; // May be undefined
    const currentTier = account?.tier; // May be undefined
    const currentInterval = account?.billing?.interval; // May be undefined
    const currentTierCode = currentTier?.code; // May be undefined

    // Figure out buttons, labels and the submit action
    let submitAction, submitButtonLabel, banner;
    if (!account) {
        submitButtonLabel = t("account_upgrade_dialog_button_redirect_signup");
        submitAction = Action.REDIRECT_SIGNUP;
        banner = null;
    } else if (currentTierCode === newTierCode && (currentInterval === undefined || currentInterval === interval)) {
        submitButtonLabel = t("account_upgrade_dialog_button_update_subscription");
        submitAction = null;
        banner = (currentTierCode) ? Banner.PRORATION_INFO : null;
    } else if (!currentTierCode) {
        submitButtonLabel = t("account_upgrade_dialog_button_pay_now");
        submitAction = Action.CREATE_SUBSCRIPTION;
        banner = null;
    } else if (!newTierCode) {
        submitButtonLabel = t("account_upgrade_dialog_button_cancel_subscription");
        submitAction = Action.CANCEL_SUBSCRIPTION;
        banner = Banner.CANCEL_WARNING;
    } else {
        submitButtonLabel = t("account_upgrade_dialog_button_update_subscription");
        submitAction = Action.UPDATE_SUBSCRIPTION;
        banner = Banner.PRORATION_INFO;
    }

    // Exceptional conditions
    if (loading) {
        submitAction = null;
    } else if (newTier?.code && account?.reservations?.length > newTier?.limits?.reservations) {
        submitAction = null;
        banner = Banner.RESERVATIONS_WARNING;
    }

    const handleSubmit = async () => {
        if (submitAction === Action.REDIRECT_SIGNUP) {
            window.location.href = routes.signup;
            return;
        }
        try {
            setLoading(true);
            if (submitAction === Action.CREATE_SUBSCRIPTION) {
                const response = await accountApi.createBillingSubscription(newTierCode, interval);
                window.location.href = response.redirect_url;
            } else if (submitAction === Action.UPDATE_SUBSCRIPTION) {
                await accountApi.updateBillingSubscription(newTierCode, interval);
            } else if (submitAction === Action.CANCEL_SUBSCRIPTION) {
                await accountApi.deleteBillingSubscription();
            }
            props.onCancel();
        } catch (e) {
            console.log(`[UpgradeDialog] Error changing billing subscription`, e);
            if (e instanceof UnauthorizedError) {
                session.resetAndRedirect(routes.login);
            } else {
                setError(e.message);
            }
        } finally {
            setLoading(false);
        }
    }

    // Figure out discount
    let discount = 0, upto = false;
    if (newTier?.prices) {
        discount = Math.round(((newTier.prices.month*12/newTier.prices.year)-1)*100);
    } else {
        let n = 0;
        for (const t of tiers) {
            if (t.prices) {
                const tierDiscount = Math.round(((t.prices.month*12/t.prices.year)-1)*100);
                if (tierDiscount > discount) {
                    discount = tierDiscount;
                    n++;
                }
            }
        }
        upto = n > 1;
    }

    return (
        <Dialog
            open={props.open}
            onClose={props.onCancel}
            maxWidth="lg"
            fullScreen={fullScreen}
        >
            <DialogTitle>
                <div style={{ display: "flex", flexDirection: "row" }}>
                    <div style={{ flexGrow: 1 }}>{t("account_upgrade_dialog_title")}</div>
                    <div style={{
                        display: "flex",
                        flexDirection: "row",
                        alignItems: "center",
                        marginTop: "4px"
                    }}>
                        <Typography component="span" variant="subtitle1">{t("account_upgrade_dialog_interval_monthly")}</Typography>
                        <Switch
                            checked={interval === SubscriptionInterval.YEAR}
                            onChange={(ev) => setInterval(ev.target.checked ? SubscriptionInterval.YEAR : SubscriptionInterval.MONTH)}
                        />
                        <Typography component="span" variant="subtitle1">{t("account_upgrade_dialog_interval_yearly")}</Typography>
                        {discount > 0 &&
                            <Chip
                                label={upto ? t("account_upgrade_dialog_interval_yearly_discount_save_up_to", { discount: discount }) : t("account_upgrade_dialog_interval_yearly_discount_save", { discount: discount })}
                                color="primary"
                                size="small"
                                variant={interval === SubscriptionInterval.YEAR ? "filled" : "outlined"}
                                sx={{ marginLeft: "5px" }}
                            />
                        }
                    </div>
                </div>
            </DialogTitle>
            <DialogContent>
                <div style={{
                    display: "flex",
                    flexDirection: "row",
                    marginBottom: "8px",
                    width: "100%"
                }}>
                    {tiers.map(tier =>
                        <TierCard
                            key={`tierCard${tier.code || '_free'}`}
                            tier={tier}
                            current={currentTierCode === tier.code} // tier.code or currentTierCode may be undefined!
                            selected={newTierCode === tier.code} // tier.code may be undefined!
                            interval={interval}
                            onClick={() => setNewTierCode(tier.code)} // tier.code may be undefined!
                        />
                    )}
                </div>
                {banner === Banner.CANCEL_WARNING &&
                    <Alert severity="warning" sx={{ fontSize: "1rem" }}>
                        <Trans
                            i18nKey="account_upgrade_dialog_cancel_warning"
                            values={{ date: formatShortDate(account?.billing?.paid_until || 0) }} />
                    </Alert>
                }
                {banner === Banner.PRORATION_INFO &&
                    <Alert severity="info" sx={{ fontSize: "1rem" }}>
                        <Trans i18nKey="account_upgrade_dialog_proration_info" />
                    </Alert>
                }
                {banner === Banner.RESERVATIONS_WARNING &&
                    <Alert severity="warning" sx={{ fontSize: "1rem" }}>
                        <Trans
                            i18nKey="account_upgrade_dialog_reservations_warning"
                            count={account?.reservations.length - newTier?.limits.reservations}
                            components={{
                                Link: <NavLink to={routes.settings}/>,
                            }}
                        />
                    </Alert>
                }
            </DialogContent>
            <Box sx={{
                display: 'flex',
                flexDirection: 'row',
                justifyContent: 'space-between',
                paddingLeft: '24px',
                paddingBottom: '8px',
            }}>
                <DialogContentText
                    component="div"
                    aria-live="polite"
                    sx={{
                        margin: '0px',
                        paddingTop: '12px',
                        paddingBottom: '4px'
                    }}
                >
                    {config.billing_contact.indexOf('@') !== -1 &&
                        <><Trans i18nKey="account_upgrade_dialog_billing_contact_email" components={{ Link: <Link href={`mailto:${config.billing_contact}`}/> }}/>{" "}</>
                    }
                    {config.billing_contact.match(`^http?s://`) &&
                        <><Trans i18nKey="account_upgrade_dialog_billing_contact_website" components={{ Link: <Link href={config.billing_contact} target="_blank"/> }}/>{" "}</>
                    }
                    {error}
                </DialogContentText>
                <DialogActions sx={{paddingRight: 2}}>
                    <Button onClick={props.onCancel}>{t("account_upgrade_dialog_button_cancel")}</Button>
                    <Button onClick={handleSubmit} disabled={!submitAction}>{submitButtonLabel}</Button>
                </DialogActions>
            </Box>
        </Dialog>
    );
};

const TierCard = (props) => {
    const { t } = useTranslation();
    const tier = props.tier;

    let cardStyle, labelStyle, labelText;
    if (props.selected) {
        cardStyle = { background: "#eee", border: "3px solid #338574" };
        labelStyle = { background: "#338574", color: "white" };
        labelText = t("account_upgrade_dialog_tier_selected_label");
    } else if (props.current) {
        cardStyle = { border: "3px solid #eee" };
        labelStyle = { background: "#eee", color: "black" };
        labelText = t("account_upgrade_dialog_tier_current_label");
    } else {
        cardStyle = { border: "3px solid transparent" };
    }

    let monthlyPrice;
    if (!tier.prices) {
        monthlyPrice = 0;
    } else if (props.interval === SubscriptionInterval.YEAR) {
        monthlyPrice = tier.prices.year/12;
    } else if (props.interval === SubscriptionInterval.MONTH) {
        monthlyPrice = tier.prices.month;
    }

    return (
        <Box sx={{
            m: "7px",
            minWidth: "240px",
            flexGrow: 1,
            flexShrink: 1,
            flexBasis: 0,
            borderRadius: "5px",
            "&:first-of-type": { ml: 0 },
            "&:last-of-type": { mr: 0 },
            ...cardStyle
        }}>
            <Card sx={{ height: "100%" }}>
                <CardActionArea sx={{ height: "100%" }}>
                    <CardContent onClick={props.onClick} sx={{ height: "100%" }}>
                        {labelStyle &&
                            <div style={{
                                position: "absolute",
                                top: "0",
                                right: "15px",
                                padding: "2px 10px",
                                borderRadius: "3px",
                                ...labelStyle
                            }}>{labelText}</div>
                        }
                        <Typography variant="subtitle1" component="div">
                            {tier.name || t("account_basics_tier_free")}
                        </Typography>
                        <div>
                            <Typography component="span" variant="h4" sx={{ fontWeight: 500, marginRight: "3px" }}>{formatPrice(monthlyPrice)}</Typography>
                            {monthlyPrice > 0 && <>/ {t("account_upgrade_dialog_tier_price_per_month")}</>}
                        </div>
                        <List dense>
                            {tier.limits.reservations > 0 && <Feature>{t("account_upgrade_dialog_tier_features_reservations", { reservations: tier.limits.reservations, count: tier.limits.reservations })}</Feature>}
                            {tier.limits.reservations === 0 && <NoFeature>{t("account_upgrade_dialog_tier_features_no_reservations")}</NoFeature>}
                            <Feature>{t("account_upgrade_dialog_tier_features_messages", { messages: formatNumber(tier.limits.messages), count: tier.limits.messages })}</Feature>
                            <Feature>{t("account_upgrade_dialog_tier_features_emails", { emails: formatNumber(tier.limits.emails), count: tier.limits.emails })}</Feature>
                            <Feature>{t("account_upgrade_dialog_tier_features_attachment_file_size", { filesize: formatBytes(tier.limits.attachment_file_size, 0) })}</Feature>
                            <Feature>{t("account_upgrade_dialog_tier_features_attachment_total_size", { totalsize: formatBytes(tier.limits.attachment_total_size, 0) })}</Feature>
                        </List>
                        {tier.prices && props.interval === SubscriptionInterval.MONTH &&
                            <Typography variant="body2" color="gray">
                                {t("account_upgrade_dialog_tier_price_billed_monthly", { price: formatPrice(tier.prices.month*12) })}
                            </Typography>
                        }
                        {tier.prices && props.interval === SubscriptionInterval.YEAR &&
                            <Typography variant="body2" color="gray">
                                {t("account_upgrade_dialog_tier_price_billed_yearly", { price: formatPrice(tier.prices.year), save: formatPrice(tier.prices.month*12-tier.prices.year) })}
                            </Typography>
                        }
                    </CardContent>
                </CardActionArea>
            </Card>
        </Box>

    );
}

const Feature = (props) => {
    return <FeatureItem feature={true}>{props.children}</FeatureItem>;
}

const NoFeature = (props) => {
    return <FeatureItem feature={false}>{props.children}</FeatureItem>;
}

const FeatureItem = (props) => {
    return (
        <ListItem disableGutters sx={{m: 0, p: 0}}>
            <ListItemIcon sx={{minWidth: "24px"}}>
                {props.feature && <Check fontSize="small" sx={{ color: "#338574" }}/>}
                {!props.feature && <Close fontSize="small" sx={{ color: "gray" }}/>}
            </ListItemIcon>
            <ListItemText
                sx={{mt: "2px", mb: "2px"}}
                primary={
                    <Typography variant="body1">
                        {props.children}
                    </Typography>
                }
            />
        </ListItem>

    );
};

const Action = {
    REDIRECT_SIGNUP: 1,
    CREATE_SUBSCRIPTION: 2,
    UPDATE_SUBSCRIPTION: 3,
    CANCEL_SUBSCRIPTION: 4
};

const Banner = {
    CANCEL_WARNING: 1,
    PRORATION_INFO: 2,
    RESERVATIONS_WARNING: 3
};

export default UpgradeDialog;
