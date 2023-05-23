import * as React from "react";
import { useContext, useState } from "react";
import {
  Alert,
  CardActions,
  CardContent,
  Chip,
  FormControl,
  FormControlLabel,
  LinearProgress,
  Link,
  Portal,
  Radio,
  RadioGroup,
  Select,
  Snackbar,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  useMediaQuery,
} from "@mui/material";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import EditIcon from "@mui/icons-material/Edit";
import Container from "@mui/material/Container";
import Card from "@mui/material/Card";
import Button from "@mui/material/Button";
import { Trans, useTranslation } from "react-i18next";
import session from "../app/Session";
import DeleteOutlineIcon from "@mui/icons-material/DeleteOutline";
import theme from "./theme";
import Dialog from "@mui/material/Dialog";
import DialogTitle from "@mui/material/DialogTitle";
import DialogContent from "@mui/material/DialogContent";
import TextField from "@mui/material/TextField";
import routes from "./routes";
import IconButton from "@mui/material/IconButton";
import {
  formatBytes,
  formatShortDate,
  formatShortDateTime,
  openUrl,
} from "../app/utils";
import accountApi, {
  LimitBasis,
  Role,
  SubscriptionInterval,
  SubscriptionStatus,
} from "../app/AccountApi";
import InfoOutlinedIcon from "@mui/icons-material/InfoOutlined";
import { Pref, PrefGroup } from "./Pref";
import db from "../app/db";
import i18n from "i18next";
import humanizeDuration from "humanize-duration";
import UpgradeDialog from "./UpgradeDialog";
import CelebrationIcon from "@mui/icons-material/Celebration";
import { AccountContext } from "./App";
import DialogFooter from "./DialogFooter";
import { Paragraph } from "./styles";
import CloseIcon from "@mui/icons-material/Close";
import { ContentCopy, Public } from "@mui/icons-material";
import MenuItem from "@mui/material/MenuItem";
import DialogContentText from "@mui/material/DialogContentText";
import { IncorrectPasswordError, UnauthorizedError } from "../app/errors";
import { ProChip } from "./SubscriptionPopup";
import AddIcon from "@mui/icons-material/Add";

const Account = () => {
  if (!session.exists()) {
    window.location.href = routes.app;
    return <></>;
  }
  return (
    <Container maxWidth="md" sx={{ marginTop: 3, marginBottom: 3 }}>
      <Stack spacing={3}>
        <Basics />
        <Stats />
        <Tokens />
        <Delete />
      </Stack>
    </Container>
  );
};

const Basics = () => {
  const { t } = useTranslation();
  return (
    <Card sx={{ p: 3 }} aria-label={t("account_basics_title")}>
      <Typography variant="h5" sx={{ marginBottom: 2 }}>
        {t("account_basics_title")}
      </Typography>
      <PrefGroup>
        <Username />
        <ChangePassword />
        <PhoneNumbers />
        <AccountType />
      </PrefGroup>
    </Card>
  );
};

const Username = () => {
  const { t } = useTranslation();
  const { account } = useContext(AccountContext);
  const labelId = "prefUsername";

  return (
    <Pref
      labelId={labelId}
      title={t("account_basics_username_title")}
      description={t("account_basics_username_description")}
    >
      <div aria-labelledby={labelId}>
        {session.username()}
        {account?.role === Role.ADMIN ? (
          <>
            {" "}
            <Tooltip title={t("account_basics_username_admin_tooltip")}>
              <span style={{ cursor: "default" }}>👑</span>
            </Tooltip>
          </>
        ) : (
          ""
        )}
      </div>
    </Pref>
  );
};

const ChangePassword = () => {
  const { t } = useTranslation();
  const [dialogKey, setDialogKey] = useState(0);
  const [dialogOpen, setDialogOpen] = useState(false);
  const labelId = "prefChangePassword";

  const handleDialogOpen = () => {
    setDialogKey((prev) => prev + 1);
    setDialogOpen(true);
  };

  const handleDialogClose = () => {
    setDialogOpen(false);
  };

  return (
    <Pref
      labelId={labelId}
      title={t("account_basics_password_title")}
      description={t("account_basics_password_description")}
    >
      <div aria-labelledby={labelId}>
        <Typography
          color="gray"
          sx={{ float: "left", fontSize: "0.7rem", lineHeight: "3.5" }}
        >
          ⬤⬤⬤⬤⬤⬤⬤⬤⬤⬤
        </Typography>
        <IconButton
          onClick={handleDialogOpen}
          aria-label={t("account_basics_password_description")}
        >
          <EditIcon />
        </IconButton>
      </div>
      <ChangePasswordDialog
        key={`changePasswordDialog${dialogKey}`}
        open={dialogOpen}
        onClose={handleDialogClose}
      />
    </Pref>
  );
};

const ChangePasswordDialog = (props) => {
  const { t } = useTranslation();
  const [error, setError] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));

  const handleDialogSubmit = async () => {
    try {
      console.debug(`[Account] Changing password`);
      await accountApi.changePassword(currentPassword, newPassword);
      props.onClose();
    } catch (e) {
      console.log(`[Account] Error changing password`, e);
      if (e instanceof IncorrectPasswordError) {
        setError(
          t("account_basics_password_dialog_current_password_incorrect")
        );
      } else if (e instanceof UnauthorizedError) {
        session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
      }
    }
  };

  return (
    <Dialog open={props.open} onClose={props.onCancel} fullScreen={fullScreen}>
      <DialogTitle>{t("account_basics_password_dialog_title")}</DialogTitle>
      <DialogContent>
        <TextField
          margin="dense"
          id="current-password"
          label={t("account_basics_password_dialog_current_password_label")}
          aria-label={t(
            "account_basics_password_dialog_current_password_label"
          )}
          type="password"
          value={currentPassword}
          onChange={(ev) => setCurrentPassword(ev.target.value)}
          fullWidth
          variant="standard"
        />
        <TextField
          margin="dense"
          id="new-password"
          label={t("account_basics_password_dialog_new_password_label")}
          aria-label={t("account_basics_password_dialog_new_password_label")}
          type="password"
          value={newPassword}
          onChange={(ev) => setNewPassword(ev.target.value)}
          fullWidth
          variant="standard"
        />
        <TextField
          margin="dense"
          id="confirm"
          label={t("account_basics_password_dialog_confirm_password_label")}
          aria-label={t(
            "account_basics_password_dialog_confirm_password_label"
          )}
          type="password"
          value={confirmPassword}
          onChange={(ev) => setConfirmPassword(ev.target.value)}
          fullWidth
          variant="standard"
        />
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={props.onClose}>{t("common_cancel")}</Button>
        <Button
          onClick={handleDialogSubmit}
          disabled={
            newPassword.length === 0 ||
            currentPassword.length === 0 ||
            newPassword !== confirmPassword
          }
        >
          {t("account_basics_password_dialog_button_submit")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

const AccountType = () => {
  const { t } = useTranslation();
  const { account } = useContext(AccountContext);
  const [upgradeDialogKey, setUpgradeDialogKey] = useState(0);
  const [upgradeDialogOpen, setUpgradeDialogOpen] = useState(false);
  const [showPortalError, setShowPortalError] = useState(false);

  if (!account) {
    return <></>;
  }

  const handleUpgradeClick = () => {
    setUpgradeDialogKey((k) => k + 1);
    setUpgradeDialogOpen(true);
  };

  const handleManageBilling = async () => {
    try {
      const response = await accountApi.createBillingPortalSession();
      window.open(response.redirect_url, "billing_portal");
    } catch (e) {
      console.log(`[Account] Error opening billing portal`, e);
      if (e instanceof UnauthorizedError) {
        session.resetAndRedirect(routes.login);
      } else {
        setShowPortalError(true);
      }
    }
  };

  let accountType;
  if (account.role === Role.ADMIN) {
    const tierSuffix = account.tier
      ? t("account_basics_tier_admin_suffix_with_tier", {
          tier: account.tier.name,
        })
      : t("account_basics_tier_admin_suffix_no_tier");
    accountType = `${t("account_basics_tier_admin")} ${tierSuffix}`;
  } else if (!account.tier) {
    accountType = config.enable_payments
      ? t("account_basics_tier_free")
      : t("account_basics_tier_basic");
  } else {
    accountType = account.tier.name;
    if (account.billing?.interval === SubscriptionInterval.MONTH) {
      accountType += ` (${t("account_basics_tier_interval_monthly")})`;
    } else if (account.billing?.interval === SubscriptionInterval.YEAR) {
      accountType += ` (${t("account_basics_tier_interval_yearly")})`;
    }
  }

  return (
    <Pref
      alignTop={
        account.billing?.status === SubscriptionStatus.PAST_DUE ||
        account.billing?.cancel_at > 0
      }
      title={t("account_basics_tier_title")}
      description={t("account_basics_tier_description")}
    >
      <div>
        {accountType}
        {account.billing?.paid_until && !account.billing?.cancel_at && (
          <Tooltip
            title={t("account_basics_tier_paid_until", {
              date: formatShortDate(account.billing?.paid_until),
            })}
          >
            <span>
              <InfoIcon />
            </span>
          </Tooltip>
        )}
        {config.enable_payments &&
          account.role === Role.USER &&
          !account.billing?.subscription && (
            <Button
              variant="outlined"
              size="small"
              startIcon={<CelebrationIcon sx={{ color: "#55b86e" }} />}
              onClick={handleUpgradeClick}
              sx={{ ml: 1 }}
            >
              {t("account_basics_tier_upgrade_button")}
            </Button>
          )}
        {config.enable_payments &&
          account.role === Role.USER &&
          account.billing?.subscription && (
            <Button
              variant="outlined"
              size="small"
              onClick={handleUpgradeClick}
              sx={{ ml: 1 }}
            >
              {t("account_basics_tier_change_button")}
            </Button>
          )}
        {config.enable_payments &&
          account.role === Role.USER &&
          account.billing?.customer && (
            <Button
              variant="outlined"
              size="small"
              onClick={handleManageBilling}
              sx={{ ml: 1 }}
            >
              {t("account_basics_tier_manage_billing_button")}
            </Button>
          )}
        {config.enable_payments && (
          <UpgradeDialog
            key={`upgradeDialogFromAccount${upgradeDialogKey}`}
            open={upgradeDialogOpen}
            onCancel={() => setUpgradeDialogOpen(false)}
          />
        )}
      </div>
      {account.billing?.status === SubscriptionStatus.PAST_DUE && (
        <Alert severity="error" sx={{ mt: 1 }}>
          {t("account_basics_tier_payment_overdue")}
        </Alert>
      )}
      {account.billing?.cancel_at > 0 && (
        <Alert severity="warning" sx={{ mt: 1 }}>
          {t("account_basics_tier_canceled_subscription", {
            date: formatShortDate(account.billing.cancel_at),
          })}
        </Alert>
      )}
      <Portal>
        <Snackbar
          open={showPortalError}
          autoHideDuration={3000}
          onClose={() => setShowPortalError(false)}
          message={t("account_usage_cannot_create_portal_session")}
        />
      </Portal>
    </Pref>
  );
};

const PhoneNumbers = () => {
  const { t } = useTranslation();
  const { account } = useContext(AccountContext);
  const [dialogKey, setDialogKey] = useState(0);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [snackOpen, setSnackOpen] = useState(false);
  const labelId = "prefPhoneNumbers";

  const handleDialogOpen = () => {
    setDialogKey((prev) => prev + 1);
    setDialogOpen(true);
  };

  const handleDialogClose = () => {
    setDialogOpen(false);
  };

  const handleCopy = (phoneNumber) => {
    navigator.clipboard.writeText(phoneNumber);
    setSnackOpen(true);
  };

  const handleDelete = async (phoneNumber) => {
    try {
      await accountApi.deletePhoneNumber(phoneNumber);
    } catch (e) {
      console.log(`[Account] Error deleting phone number`, e);
      if (e instanceof UnauthorizedError) {
        session.resetAndRedirect(routes.login);
      }
    }
  };

  if (!config.enable_calls) {
    return null;
  }

  if (account?.limits.calls === 0) {
    return (
      <Pref
        title={
          <>
            {t("account_basics_phone_numbers_title")}
            {config.enable_payments && <ProChip />}
          </>
        }
        description={t("account_basics_phone_numbers_description")}
      >
        <em>{t("account_usage_calls_none")}</em>
      </Pref>
    );
  }

  return (
    <Pref
      labelId={labelId}
      title={t("account_basics_phone_numbers_title")}
      description={t("account_basics_phone_numbers_description")}
    >
      <div aria-labelledby={labelId}>
        {account?.phone_numbers?.map((phoneNumber) => (
          <Chip
            label={
              <Tooltip title={t("common_copy_to_clipboard")}>
                <span>{phoneNumber}</span>
              </Tooltip>
            }
            variant="outlined"
            onClick={() => handleCopy(phoneNumber)}
            onDelete={() => handleDelete(phoneNumber)}
          />
        ))}
        {!account?.phone_numbers && (
          <em>{t("account_basics_phone_numbers_no_phone_numbers_yet")}</em>
        )}
        <IconButton onClick={handleDialogOpen}>
          <AddIcon />
        </IconButton>
      </div>
      <AddPhoneNumberDialog
        key={`addPhoneNumberDialog${dialogKey}`}
        open={dialogOpen}
        onClose={handleDialogClose}
      />
      <Portal>
        <Snackbar
          open={snackOpen}
          autoHideDuration={3000}
          onClose={() => setSnackOpen(false)}
          message={t("account_basics_phone_numbers_copied_to_clipboard")}
        />
      </Portal>
    </Pref>
  );
};

const AddPhoneNumberDialog = (props) => {
  const { t } = useTranslation();
  const [error, setError] = useState("");
  const [phoneNumber, setPhoneNumber] = useState("");
  const [channel, setChannel] = useState("sms");
  const [code, setCode] = useState("");
  const [sending, setSending] = useState(false);
  const [verificationCodeSent, setVerificationCodeSent] = useState(false);
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));

  const handleDialogSubmit = async () => {
    if (!verificationCodeSent) {
      await verifyPhone();
    } else {
      await checkVerifyPhone();
    }
  };

  const handleCancel = () => {
    if (verificationCodeSent) {
      setVerificationCodeSent(false);
      setCode("");
    } else {
      props.onClose();
    }
  };

  const verifyPhone = async () => {
    try {
      setSending(true);
      await accountApi.verifyPhoneNumber(phoneNumber, channel);
      setVerificationCodeSent(true);
    } catch (e) {
      console.log(`[Account] Error sending verification`, e);
      if (e instanceof UnauthorizedError) {
        session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
      }
    } finally {
      setSending(false);
    }
  };

  const checkVerifyPhone = async () => {
    try {
      setSending(true);
      await accountApi.addPhoneNumber(phoneNumber, code);
      props.onClose();
    } catch (e) {
      console.log(`[Account] Error confirming verification`, e);
      if (e instanceof UnauthorizedError) {
        session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
      }
    } finally {
      setSending(false);
    }
  };

  return (
    <Dialog open={props.open} onClose={props.onCancel} fullScreen={fullScreen}>
      <DialogTitle>
        {t("account_basics_phone_numbers_dialog_title")}
      </DialogTitle>
      <DialogContent>
        <DialogContentText>
          {t("account_basics_phone_numbers_dialog_description")}
        </DialogContentText>
        {!verificationCodeSent && (
          <div style={{ display: "flex" }}>
            <TextField
              margin="dense"
              label={t("account_basics_phone_numbers_dialog_number_label")}
              aria-label={t("account_basics_phone_numbers_dialog_number_label")}
              placeholder={t(
                "account_basics_phone_numbers_dialog_number_placeholder"
              )}
              type="tel"
              value={phoneNumber}
              onChange={(ev) => setPhoneNumber(ev.target.value)}
              inputProps={{ inputMode: "tel", pattern: "+[0-9]*" }}
              variant="standard"
              sx={{ flexGrow: 1 }}
            />
            <FormControl sx={{ flexWrap: "nowrap" }}>
              <RadioGroup
                row
                sx={{ flexGrow: 1, marginTop: "8px", marginLeft: "5px" }}
              >
                <FormControlLabel
                  value="sms"
                  control={
                    <Radio
                      checked={channel === "sms"}
                      onChange={(e) => setChannel(e.target.value)}
                    />
                  }
                  label={t("account_basics_phone_numbers_dialog_channel_sms")}
                />
                <FormControlLabel
                  value="call"
                  control={
                    <Radio
                      checked={channel === "call"}
                      onChange={(e) => setChannel(e.target.value)}
                    />
                  }
                  label={t("account_basics_phone_numbers_dialog_channel_call")}
                  sx={{ marginRight: 0 }}
                />
              </RadioGroup>
            </FormControl>
          </div>
        )}
        {verificationCodeSent && (
          <TextField
            margin="dense"
            label={t("account_basics_phone_numbers_dialog_code_label")}
            aria-label={t("account_basics_phone_numbers_dialog_code_label")}
            placeholder={t(
              "account_basics_phone_numbers_dialog_code_placeholder"
            )}
            type="text"
            value={code}
            onChange={(ev) => setCode(ev.target.value)}
            fullWidth
            inputProps={{ inputMode: "numeric", pattern: "[0-9]*" }}
            variant="standard"
          />
        )}
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={handleCancel}>
          {verificationCodeSent ? t("common_back") : t("common_cancel")}
        </Button>
        <Button
          onClick={handleDialogSubmit}
          disabled={sending || !/^\+\d+$/.test(phoneNumber)}
        >
          {!verificationCodeSent &&
            channel === "sms" &&
            t("account_basics_phone_numbers_dialog_verify_button_sms")}
          {!verificationCodeSent &&
            channel === "call" &&
            t("account_basics_phone_numbers_dialog_verify_button_call")}
          {verificationCodeSent &&
            t("account_basics_phone_numbers_dialog_check_verification_button")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

const Stats = () => {
  const { t } = useTranslation();
  const { account } = useContext(AccountContext);

  if (!account) {
    return <></>;
  }

  const normalize = (value, max) => {
    return Math.min((value / max) * 100, 100);
  };

  return (
    <Card sx={{ p: 3 }} aria-label={t("account_usage_title")}>
      <Typography variant="h5" sx={{ marginBottom: 2 }}>
        {t("account_usage_title")}
      </Typography>
      <PrefGroup>
        {(account.role === Role.ADMIN || account.limits.reservations > 0) && (
          <Pref title={t("account_usage_reservations_title")}>
            <div>
              <Typography variant="body2" sx={{ float: "left" }}>
                {account.stats.reservations.toLocaleString()}
              </Typography>
              <Typography variant="body2" sx={{ float: "right" }}>
                {account.role === Role.USER
                  ? t("account_usage_of_limit", {
                      limit: account.limits.reservations.toLocaleString(),
                    })
                  : t("account_usage_unlimited")}
              </Typography>
            </div>
            <LinearProgress
              variant="determinate"
              value={
                account.role === Role.USER && account.limits.reservations > 0
                  ? normalize(
                      account.stats.reservations,
                      account.limits.reservations
                    )
                  : 100
              }
            />
          </Pref>
        )}
        <Pref
          title={
            <>
              {t("account_usage_messages_title")}
              <Tooltip title={t("account_usage_limits_reset_daily")}>
                <span>
                  <InfoIcon />
                </span>
              </Tooltip>
            </>
          }
        >
          <div>
            <Typography variant="body2" sx={{ float: "left" }}>
              {account.stats.messages.toLocaleString()}
            </Typography>
            <Typography variant="body2" sx={{ float: "right" }}>
              {account.role === Role.USER
                ? t("account_usage_of_limit", {
                    limit: account.limits.messages.toLocaleString(),
                  })
                : t("account_usage_unlimited")}
            </Typography>
          </div>
          <LinearProgress
            variant="determinate"
            value={
              account.role === Role.USER
                ? normalize(account.stats.messages, account.limits.messages)
                : 100
            }
          />
        </Pref>
        {config.enable_emails && (
          <Pref
            title={
              <>
                {t("account_usage_emails_title")}
                <Tooltip title={t("account_usage_limits_reset_daily")}>
                  <span>
                    <InfoIcon />
                  </span>
                </Tooltip>
              </>
            }
          >
            <div>
              <Typography variant="body2" sx={{ float: "left" }}>
                {account.stats.emails.toLocaleString()}
              </Typography>
              <Typography variant="body2" sx={{ float: "right" }}>
                {account.role === Role.USER
                  ? t("account_usage_of_limit", {
                      limit: account.limits.emails.toLocaleString(),
                    })
                  : t("account_usage_unlimited")}
              </Typography>
            </div>
            <LinearProgress
              variant="determinate"
              value={
                account.role === Role.USER
                  ? normalize(account.stats.emails, account.limits.emails)
                  : 100
              }
            />
          </Pref>
        )}
        {config.enable_calls &&
          (account.role === Role.ADMIN || account.limits.calls > 0) && (
            <Pref
              title={
                <>
                  {t("account_usage_calls_title")}
                  <Tooltip title={t("account_usage_limits_reset_daily")}>
                    <span>
                      <InfoIcon />
                    </span>
                  </Tooltip>
                </>
              }
            >
              <div>
                <Typography variant="body2" sx={{ float: "left" }}>
                  {account.stats.calls.toLocaleString()}
                </Typography>
                <Typography variant="body2" sx={{ float: "right" }}>
                  {account.role === Role.USER
                    ? t("account_usage_of_limit", {
                        limit: account.limits.calls.toLocaleString(),
                      })
                    : t("account_usage_unlimited")}
                </Typography>
              </div>
              <LinearProgress
                variant="determinate"
                value={
                  account.role === Role.USER && account.limits.calls > 0
                    ? normalize(account.stats.calls, account.limits.calls)
                    : 100
                }
              />
            </Pref>
          )}
        <Pref
          alignTop
          title={t("account_usage_attachment_storage_title")}
          description={t("account_usage_attachment_storage_description", {
            filesize: formatBytes(account.limits.attachment_file_size),
            expiry: humanizeDuration(
              account.limits.attachment_expiry_duration * 1000,
              {
                language: i18n.resolvedLanguage,
                fallbacks: ["en"],
              }
            ),
          })}
        >
          <div>
            <Typography variant="body2" sx={{ float: "left" }}>
              {formatBytes(account.stats.attachment_total_size)}
            </Typography>
            <Typography variant="body2" sx={{ float: "right" }}>
              {account.role === Role.USER
                ? t("account_usage_of_limit", {
                    limit: formatBytes(account.limits.attachment_total_size),
                  })
                : t("account_usage_unlimited")}
            </Typography>
          </div>
          <LinearProgress
            variant="determinate"
            value={
              account.role === Role.USER
                ? normalize(
                    account.stats.attachment_total_size,
                    account.limits.attachment_total_size
                  )
                : 100
            }
          />
        </Pref>
        {config.enable_reservations &&
          account.role === Role.USER &&
          account.limits.reservations === 0 && (
            <Pref
              title={
                <>
                  {t("account_usage_reservations_title")}
                  {config.enable_payments && <ProChip />}
                </>
              }
            >
              <em>{t("account_usage_reservations_none")}</em>
            </Pref>
          )}
        {config.enable_calls &&
          account.role === Role.USER &&
          account.limits.calls === 0 && (
            <Pref
              title={
                <>
                  {t("account_usage_calls_title")}
                  {config.enable_payments && <ProChip />}
                </>
              }
            >
              <em>{t("account_usage_calls_none")}</em>
            </Pref>
          )}
      </PrefGroup>
      {account.role === Role.USER && account.limits.basis === LimitBasis.IP && (
        <Typography variant="body1">
          {t("account_usage_basis_ip_description")}
        </Typography>
      )}
    </Card>
  );
};

const InfoIcon = () => {
  return (
    <InfoOutlinedIcon
      sx={{
        verticalAlign: "middle",
        width: "18px",
        marginLeft: "4px",
        color: "gray",
      }}
    />
  );
};

const Tokens = () => {
  const { t } = useTranslation();
  const { account } = useContext(AccountContext);
  const [dialogKey, setDialogKey] = useState(0);
  const [dialogOpen, setDialogOpen] = useState(false);
  const tokens = account?.tokens || [];

  const handleCreateClick = () => {
    setDialogKey((prev) => prev + 1);
    setDialogOpen(true);
  };

  const handleDialogClose = () => {
    setDialogOpen(false);
  };

  const handleDialogSubmit = async (user) => {
    setDialogOpen(false);
    //
  };
  return (
    <Card sx={{ padding: 1 }} aria-label={t("prefs_users_title")}>
      <CardContent sx={{ paddingBottom: 1 }}>
        <Typography variant="h5" sx={{ marginBottom: 2 }}>
          {t("account_tokens_title")}
        </Typography>
        <Paragraph>
          <Trans
            i18nKey="account_tokens_description"
            components={{
              Link: <Link href="/docs/publish/#access-tokens" />,
            }}
          />
        </Paragraph>
        {tokens?.length > 0 && <TokensTable tokens={tokens} />}
      </CardContent>
      <CardActions>
        <Button onClick={handleCreateClick}>
          {t("account_tokens_table_create_token_button")}
        </Button>
      </CardActions>
      <TokenDialog
        key={`tokenDialogCreate${dialogKey}`}
        open={dialogOpen}
        onClose={handleDialogClose}
      />
    </Card>
  );
};

const TokensTable = (props) => {
  const { t } = useTranslation();
  const [snackOpen, setSnackOpen] = useState(false);
  const [upsertDialogKey, setUpsertDialogKey] = useState(0);
  const [upsertDialogOpen, setUpsertDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [selectedToken, setSelectedToken] = useState(null);

  const tokens = (props.tokens || []).sort((a, b) => {
    if (a.token === session.token()) {
      return -1;
    } else if (b.token === session.token()) {
      return 1;
    }
    return a.token.localeCompare(b.token);
  });

  const handleEditClick = (token) => {
    setUpsertDialogKey((prev) => prev + 1);
    setSelectedToken(token);
    setUpsertDialogOpen(true);
  };

  const handleDialogClose = () => {
    setUpsertDialogOpen(false);
    setDeleteDialogOpen(false);
    setSelectedToken(null);
  };

  const handleDeleteClick = async (token) => {
    setSelectedToken(token);
    setDeleteDialogOpen(true);
  };

  const handleCopy = async (token) => {
    await navigator.clipboard.writeText(token);
    setSnackOpen(true);
  };

  return (
    <Table size="small" aria-label={t("account_tokens_title")}>
      <TableHead>
        <TableRow>
          <TableCell sx={{ paddingLeft: 0 }}>
            {t("account_tokens_table_token_header")}
          </TableCell>
          <TableCell>{t("account_tokens_table_label_header")}</TableCell>
          <TableCell>{t("account_tokens_table_expires_header")}</TableCell>
          <TableCell>{t("account_tokens_table_last_access_header")}</TableCell>
          <TableCell />
        </TableRow>
      </TableHead>
      <TableBody>
        {tokens.map((token) => (
          <TableRow
            key={token.token}
            sx={{ "&:last-child td, &:last-child th": { border: 0 } }}
          >
            <TableCell
              component="th"
              scope="row"
              sx={{ paddingLeft: 0, whiteSpace: "nowrap" }}
              aria-label={t("account_tokens_table_token_header")}
            >
              <span>
                <span style={{ fontFamily: "Monospace", fontSize: "0.9rem" }}>
                  {token.token.slice(0, 12)}
                </span>
                ...
                <Tooltip
                  title={t("common_copy_to_clipboard")}
                  placement="right"
                >
                  <IconButton onClick={() => handleCopy(token.token)}>
                    <ContentCopy />
                  </IconButton>
                </Tooltip>
              </span>
            </TableCell>
            <TableCell aria-label={t("account_tokens_table_label_header")}>
              {token.token === session.token() && (
                <em>{t("account_tokens_table_current_session")}</em>
              )}
              {token.token !== session.token() && (token.label || "-")}
            </TableCell>
            <TableCell
              sx={{ whiteSpace: "nowrap" }}
              aria-label={t("account_tokens_table_expires_header")}
            >
              {token.expires ? (
                formatShortDateTime(token.expires)
              ) : (
                <em>{t("account_tokens_table_never_expires")}</em>
              )}
            </TableCell>
            <TableCell
              sx={{ whiteSpace: "nowrap" }}
              aria-label={t("account_tokens_table_last_access_header")}
            >
              <div style={{ display: "flex", alignItems: "center" }}>
                <span>{formatShortDateTime(token.last_access)}</span>
                <Tooltip
                  title={t("account_tokens_table_last_origin_tooltip", {
                    ip: token.last_origin,
                  })}
                >
                  <IconButton
                    onClick={() =>
                      openUrl(
                        `https://whatismyipaddress.com/ip/${token.last_origin}`
                      )
                    }
                  >
                    <Public />
                  </IconButton>
                </Tooltip>
              </div>
            </TableCell>
            <TableCell align="right" sx={{ whiteSpace: "nowrap" }}>
              {token.token !== session.token() && (
                <>
                  <IconButton
                    onClick={() => handleEditClick(token)}
                    aria-label={t("account_tokens_dialog_title_edit")}
                  >
                    <EditIcon />
                  </IconButton>
                  <IconButton
                    onClick={() => handleDeleteClick(token)}
                    aria-label={t("account_tokens_dialog_title_delete")}
                  >
                    <CloseIcon />
                  </IconButton>
                </>
              )}
              {token.token === session.token() && (
                <Tooltip
                  title={t("account_tokens_table_cannot_delete_or_edit")}
                >
                  <span>
                    <IconButton disabled>
                      <EditIcon />
                    </IconButton>
                    <IconButton disabled>
                      <CloseIcon />
                    </IconButton>
                  </span>
                </Tooltip>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
      <Portal>
        <Snackbar
          open={snackOpen}
          autoHideDuration={3000}
          onClose={() => setSnackOpen(false)}
          message={t("account_tokens_table_copied_to_clipboard")}
        />
      </Portal>
      <TokenDialog
        key={`tokenDialogEdit${upsertDialogKey}`}
        open={upsertDialogOpen}
        token={selectedToken}
        onClose={handleDialogClose}
      />
      <TokenDeleteDialog
        open={deleteDialogOpen}
        token={selectedToken}
        onClose={handleDialogClose}
      />
    </Table>
  );
};

const TokenDialog = (props) => {
  const { t } = useTranslation();
  const [error, setError] = useState("");
  const [label, setLabel] = useState(props.token?.label || "");
  const [expires, setExpires] = useState(props.token ? -1 : 0);
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));
  const editMode = !!props.token;

  const handleSubmit = async () => {
    try {
      if (editMode) {
        await accountApi.updateToken(props.token.token, label, expires);
      } else {
        await accountApi.createToken(label, expires);
      }
      props.onClose();
    } catch (e) {
      console.log(`[Account] Error creating token`, e);
      if (e instanceof UnauthorizedError) {
        session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
      }
    }
  };

  return (
    <Dialog
      open={props.open}
      onClose={props.onClose}
      maxWidth="sm"
      fullWidth
      fullScreen={fullScreen}
    >
      <DialogTitle>
        {editMode
          ? t("account_tokens_dialog_title_edit")
          : t("account_tokens_dialog_title_create")}
      </DialogTitle>
      <DialogContent>
        <TextField
          margin="dense"
          id="token-label"
          label={t("account_tokens_dialog_label")}
          aria-label={t("account_delete_dialog_label")}
          type="text"
          value={label}
          onChange={(ev) => setLabel(ev.target.value)}
          fullWidth
          variant="standard"
        />
        <FormControl fullWidth variant="standard" sx={{ mt: 1 }}>
          <Select
            value={expires}
            onChange={(ev) => setExpires(ev.target.value)}
            aria-label={t("account_tokens_dialog_expires_label")}
          >
            {editMode && (
              <MenuItem value={-1}>
                {t("account_tokens_dialog_expires_unchanged")}
              </MenuItem>
            )}
            <MenuItem value={0}>
              {t("account_tokens_dialog_expires_never")}
            </MenuItem>
            <MenuItem value={21600}>
              {t("account_tokens_dialog_expires_x_hours", { hours: 6 })}
            </MenuItem>
            <MenuItem value={43200}>
              {t("account_tokens_dialog_expires_x_hours", { hours: 12 })}
            </MenuItem>
            <MenuItem value={259200}>
              {t("account_tokens_dialog_expires_x_days", { days: 3 })}
            </MenuItem>
            <MenuItem value={604800}>
              {t("account_tokens_dialog_expires_x_days", { days: 7 })}
            </MenuItem>
            <MenuItem value={2592000}>
              {t("account_tokens_dialog_expires_x_days", { days: 30 })}
            </MenuItem>
            <MenuItem value={7776000}>
              {t("account_tokens_dialog_expires_x_days", { days: 90 })}
            </MenuItem>
            <MenuItem value={15552000}>
              {t("account_tokens_dialog_expires_x_days", { days: 180 })}
            </MenuItem>
          </Select>
        </FormControl>
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={props.onClose}>
          {t("account_tokens_dialog_button_cancel")}
        </Button>
        <Button onClick={handleSubmit}>
          {editMode
            ? t("account_tokens_dialog_button_update")
            : t("account_tokens_dialog_button_create")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

const TokenDeleteDialog = (props) => {
  const { t } = useTranslation();
  const [error, setError] = useState("");

  const handleSubmit = async () => {
    try {
      await accountApi.deleteToken(props.token.token);
      props.onClose();
    } catch (e) {
      console.log(`[Account] Error deleting token`, e);
      if (e instanceof UnauthorizedError) {
        session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
      }
    }
  };

  return (
    <Dialog open={props.open} onClose={props.onClose}>
      <DialogTitle>{t("account_tokens_delete_dialog_title")}</DialogTitle>
      <DialogContent>
        <DialogContentText>
          <Trans i18nKey="account_tokens_delete_dialog_description" />
        </DialogContentText>
      </DialogContent>
      <DialogFooter status>
        <Button onClick={props.onClose}>{t("common_cancel")}</Button>
        <Button onClick={handleSubmit} color="error">
          {t("account_tokens_delete_dialog_submit_button")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

const Delete = () => {
  const { t } = useTranslation();
  return (
    <Card sx={{ p: 3 }} aria-label={t("account_delete_title")}>
      <Typography variant="h5" sx={{ marginBottom: 2 }}>
        {t("account_delete_title")}
      </Typography>
      <PrefGroup>
        <DeleteAccount />
      </PrefGroup>
    </Card>
  );
};

const DeleteAccount = () => {
  const { t } = useTranslation();
  const [dialogKey, setDialogKey] = useState(0);
  const [dialogOpen, setDialogOpen] = useState(false);

  const handleDialogOpen = () => {
    setDialogKey((prev) => prev + 1);
    setDialogOpen(true);
  };

  const handleDialogClose = () => {
    setDialogOpen(false);
  };

  return (
    <Pref
      title={t("account_delete_title")}
      description={t("account_delete_description")}
    >
      <div>
        <Button
          fullWidth={false}
          variant="outlined"
          color="error"
          startIcon={<DeleteOutlineIcon />}
          onClick={handleDialogOpen}
        >
          {t("account_delete_title")}
        </Button>
      </div>
      <DeleteAccountDialog
        key={`deleteAccountDialog${dialogKey}`}
        open={dialogOpen}
        onClose={handleDialogClose}
      />
    </Pref>
  );
};

const DeleteAccountDialog = (props) => {
  const { t } = useTranslation();
  const { account } = useContext(AccountContext);
  const [error, setError] = useState("");
  const [password, setPassword] = useState("");
  const fullScreen = useMediaQuery(theme.breakpoints.down("sm"));

  const handleSubmit = async () => {
    try {
      await accountApi.delete(password);
      await db.delete();
      console.debug(`[Account] Account deleted`);
      session.resetAndRedirect(routes.app);
    } catch (e) {
      console.log(`[Account] Error deleting account`, e);
      if (e instanceof IncorrectPasswordError) {
        setError(
          t("account_basics_password_dialog_current_password_incorrect")
        );
      } else if (e instanceof UnauthorizedError) {
        session.resetAndRedirect(routes.login);
      } else {
        setError(e.message);
      }
    }
  };

  return (
    <Dialog open={props.open} onClose={props.onClose} fullScreen={fullScreen}>
      <DialogTitle>{t("account_delete_title")}</DialogTitle>
      <DialogContent>
        <Typography variant="body1">
          {t("account_delete_dialog_description")}
        </Typography>
        <TextField
          margin="dense"
          id="account-delete-confirm"
          label={t("account_delete_dialog_label")}
          aria-label={t("account_delete_dialog_label")}
          type="password"
          value={password}
          onChange={(ev) => setPassword(ev.target.value)}
          fullWidth
          variant="standard"
        />
        {account?.billing?.subscription && (
          <Alert severity="warning" sx={{ mt: 1 }}>
            {t("account_delete_dialog_billing_warning")}
          </Alert>
        )}
      </DialogContent>
      <DialogFooter status={error}>
        <Button onClick={props.onClose}>
          {t("account_delete_dialog_button_cancel")}
        </Button>
        <Button
          onClick={handleSubmit}
          color="error"
          disabled={password.length === 0}
        >
          {t("account_delete_dialog_button_submit")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
};

export default Account;
