import {
  Drawer,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Toolbar,
  Divider,
  List,
  Alert,
  AlertTitle,
  Badge,
  CircularProgress,
  Link,
  ListSubheader,
  Portal,
  Tooltip,
  Typography,
  Box,
  IconButton,
  Button,
  useTheme,
} from "@mui/material";
import * as React from "react";
import { useContext, useState } from "react";
import ChatBubbleOutlineIcon from "@mui/icons-material/ChatBubbleOutline";
import Person from "@mui/icons-material/Person";
import SettingsIcon from "@mui/icons-material/Settings";
import AddIcon from "@mui/icons-material/Add";
import { useLocation, useNavigate } from "react-router-dom";
import { ChatBubble, MoreVert, NotificationsOffOutlined, Send } from "@mui/icons-material";
import ArticleIcon from "@mui/icons-material/Article";
import { Trans, useTranslation } from "react-i18next";
import CelebrationIcon from "@mui/icons-material/Celebration";
import SubscribeDialog from "./SubscribeDialog";
import { openUrl, topicDisplayName, topicUrl } from "../app/utils";
import routes from "./routes";
import { ConnectionState } from "../app/Connection";
import subscriptionManager from "../app/SubscriptionManager";
import notifier from "../app/Notifier";
import config from "../app/config";
import session from "../app/Session";
import accountApi, { Permission, Role } from "../app/AccountApi";
import UpgradeDialog from "./UpgradeDialog";
import { AccountContext } from "./App";
import { PermissionDenyAll, PermissionRead, PermissionReadWrite, PermissionWrite } from "./ReserveIcons";
import { SubscriptionPopup } from "./SubscriptionPopup";
import { useNotificationPermissionListener } from "./hooks";

const navWidth = 280;

const Navigation = (props) => {
  const navigationList = <NavList {...props} />;
  return (
    <Box component="nav" role="navigation" sx={{ width: { sm: Navigation.width }, flexShrink: { sm: 0 } }}>
      {/* Mobile drawer; only shown if menu icon clicked (mobile open) and display is small */}
      <Drawer
        variant="temporary"
        role="menubar"
        open={props.mobileDrawerOpen}
        onClose={props.onMobileDrawerToggle}
        ModalProps={{ keepMounted: true }} // Better open performance on mobile.
        sx={{
          display: { xs: "block", sm: "none" },
          "& .MuiDrawer-paper": { boxSizing: "border-box", width: navWidth, backgroundImage: "none" },
        }}
      >
        {navigationList}
      </Drawer>
      {/* Big screen drawer; persistent, shown if screen is big */}
      <Drawer
        open
        variant="permanent"
        role="menubar"
        sx={{
          display: { xs: "none", sm: "block" },
          "& .MuiDrawer-paper": { boxSizing: "border-box", width: navWidth },
        }}
      >
        {navigationList}
      </Drawer>
    </Box>
  );
};
Navigation.width = navWidth;

const NavList = (props) => {
  const theme = useTheme();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const { account } = useContext(AccountContext);
  const [subscribeDialogKey, setSubscribeDialogKey] = useState(0);
  const [subscribeDialogOpen, setSubscribeDialogOpen] = useState(false);

  const handleSubscribeReset = () => {
    setSubscribeDialogOpen(false);
    setSubscribeDialogKey((prev) => prev + 1);
  };

  const handleSubscribeSubmit = (subscription) => {
    console.log(`[Navigation] New subscription: ${subscription.id}`, subscription);
    handleSubscribeReset();
    navigate(routes.forSubscription(subscription));
  };

  const handleAccountClick = () => {
    accountApi.sync(); // Dangle!
    navigate(routes.account);
  };

  const isAdmin = account?.role === Role.ADMIN;
  const isPaid = account?.billing?.subscription;
  const showUpgradeBanner = config.enable_payments && !isAdmin && !isPaid;
  const showSubscriptionsList = props.subscriptions?.length > 0;
  const showNotificationPermissionRequired = useNotificationPermissionListener(() => notifier.notRequested());
  const showNotificationPermissionDenied = useNotificationPermissionListener(() => notifier.denied());
  const showNotificationIOSInstallRequired = notifier.iosSupportedButInstallRequired();
  const showNotificationBrowserNotSupportedBox = !showNotificationIOSInstallRequired && !notifier.browserSupported();
  const showNotificationContextNotSupportedBox = notifier.browserSupported() && !notifier.contextSupported(); // Only show if notifications are generally supported in the browser

  const alertVisible =
    showNotificationPermissionRequired ||
    showNotificationPermissionDenied ||
    showNotificationIOSInstallRequired ||
    showNotificationBrowserNotSupportedBox ||
    showNotificationContextNotSupportedBox;

  return (
    <>
      <Toolbar sx={{ display: { xs: "none", sm: "block" } }} />
      <List component="nav" sx={{ paddingTop: { xs: 0, sm: alertVisible ? 0 : "" } }}>
        {showNotificationPermissionRequired && <NotificationPermissionRequired />}
        {showNotificationPermissionDenied && <NotificationPermissionDeniedAlert />}
        {showNotificationBrowserNotSupportedBox && <NotificationBrowserNotSupportedAlert />}
        {showNotificationContextNotSupportedBox && <NotificationContextNotSupportedAlert />}
        {showNotificationIOSInstallRequired && <NotificationIOSInstallRequiredAlert />}
        {alertVisible && <Divider />}
        {!showSubscriptionsList && (session.exists() || !config.require_login) && (
          <ListItemButton onClick={() => navigate(routes.app)} selected={location.pathname === config.app_root}>
            <ListItemIcon>
              <ChatBubble />
            </ListItemIcon>
            <ListItemText primary={t("nav_button_all_notifications")} />
          </ListItemButton>
        )}
        {showSubscriptionsList && (
          <>
            <ListSubheader>{t("nav_topics_title")}</ListSubheader>
            <ListItemButton onClick={() => navigate(routes.app)} selected={location.pathname === config.app_root}>
              <ListItemIcon>
                <ChatBubble />
              </ListItemIcon>
              <ListItemText primary={t("nav_button_all_notifications")} />
            </ListItemButton>
            <SubscriptionList subscriptions={props.subscriptions} selectedSubscription={props.selectedSubscription} />
            <Divider sx={{ my: 1 }} />
          </>
        )}
        {session.exists() && (
          <ListItemButton onClick={handleAccountClick} selected={location.pathname === routes.account}>
            <ListItemIcon>
              <Person />
            </ListItemIcon>
            <ListItemText primary={t("nav_button_account")} />
          </ListItemButton>
        )}
        {session.exists() ||
          (!config.require_login && (
            <ListItemButton onClick={() => navigate(routes.settings)} selected={location.pathname === routes.settings}>
              <ListItemIcon>
                <SettingsIcon />
              </ListItemIcon>
              <ListItemText primary={t("nav_button_settings")} />
            </ListItemButton>
          ))}
        <ListItemButton onClick={() => openUrl("/docs")}>
          <ListItemIcon>
            <ArticleIcon />
          </ListItemIcon>
          <ListItemText primary={t("nav_button_documentation")} />
        </ListItemButton>
        {session.exists() ||
          (!config.require_login && (
            <ListItemButton onClick={() => props.onPublishMessageClick()}>
              <ListItemIcon>
                <Send />
              </ListItemIcon>
              <ListItemText primary={t("nav_button_publish_message")} />
            </ListItemButton>
          ))}
        {session.exists() ||
          (!config.require_login && (
            <ListItemButton onClick={() => setSubscribeDialogOpen(true)}>
              <ListItemIcon>
                <AddIcon />
              </ListItemIcon>
              <ListItemText primary={t("nav_button_subscribe")} />
            </ListItemButton>
          ))}
        {showUpgradeBanner && (
          // The text background gradient didn't seem to do well with switching between light/dark mode,
          // So adding a `key` forces React to replace the entire component when the theme changes
          <UpgradeBanner key={`upgrade-banner-${theme.palette.mode}`} mode={theme.palette.mode} />
        )}
      </List>
      <SubscribeDialog
        key={`subscribeDialog${subscribeDialogKey}`} // Resets dialog when canceled/closed
        open={subscribeDialogOpen}
        subscriptions={props.subscriptions}
        onCancel={handleSubscribeReset}
        onSuccess={handleSubscribeSubmit}
      />
    </>
  );
};

const UpgradeBanner = ({ mode }) => {
  const { t } = useTranslation();
  const [dialogKey, setDialogKey] = useState(0);
  const [dialogOpen, setDialogOpen] = useState(false);

  const handleClick = () => {
    setDialogKey((k) => k + 1);
    setDialogOpen(true);
  };

  return (
    <Box
      sx={{
        position: "fixed",
        width: `${Navigation.width - 1}px`,
        bottom: 0,
        mt: "auto",
        background:
          mode === "light"
            ? "linear-gradient(150deg, rgba(196, 228, 221, 0.46) 0%, rgb(255, 255, 255) 100%)"
            : "linear-gradient(150deg, #203631 0%, #2a6e60 100%)",
      }}
    >
      <Divider />
      <ListItemButton onClick={handleClick} sx={{ pt: 2, pb: 2 }}>
        <ListItemIcon>
          <CelebrationIcon sx={{ color: mode === "light" ? "#55b86e" : "#00ff95" }} fontSize="large" />
        </ListItemIcon>
        <ListItemText
          sx={{ ml: 1 }}
          primary={t("nav_upgrade_banner_label")}
          secondary={t("nav_upgrade_banner_description")}
          primaryTypographyProps={{
            style: {
              fontWeight: 500,
              fontSize: "1.1rem",
              background:
                mode === "light"
                  ? "-webkit-linear-gradient(45deg, #09009f, #00ff95 80%)"
                  : "-webkit-linear-gradient(45deg,rgb(255, 255, 255), #00ff95 80%)",
              WebkitBackgroundClip: "text",
              WebkitTextFillColor: "transparent",
            },
          }}
          secondaryTypographyProps={{
            style: {
              fontSize: "1rem",
            },
          }}
        />
      </ListItemButton>
      <UpgradeDialog key={`upgradeDialog${dialogKey}`} open={dialogOpen} onCancel={() => setDialogOpen(false)} />
    </Box>
  );
};

const SubscriptionList = (props) => {
  const sortedSubscriptions = props.subscriptions
    .filter((s) => !s.internal)
    .sort((a, b) => (topicUrl(a.baseUrl, a.topic) < topicUrl(b.baseUrl, b.topic) ? -1 : 1));
  return (
    <>
      {sortedSubscriptions.map((subscription) => (
        <SubscriptionItem
          key={subscription.id}
          subscription={subscription}
          selected={props.selectedSubscription && props.selectedSubscription.id === subscription.id}
        />
      ))}
    </>
  );
};

const SubscriptionItem = (props) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [menuAnchorEl, setMenuAnchorEl] = useState(null);

  const { subscription } = props;
  const iconBadge = subscription.new <= 99 ? subscription.new : "99+";
  const displayName = topicDisplayName(subscription);
  const ariaLabel = subscription.state === ConnectionState.Connecting ? `${displayName} (${t("nav_button_connecting")})` : displayName;
  const icon =
    subscription.state === ConnectionState.Connecting ? (
      <CircularProgress size="24px" />
    ) : (
      <Badge badgeContent={iconBadge} invisible={subscription.new === 0} color="primary">
        <ChatBubbleOutlineIcon />
      </Badge>
    );

  const handleClick = async () => {
    navigate(routes.forSubscription(subscription));
    await subscriptionManager.markNotificationsRead(subscription.id);
  };

  return (
    <>
      <ListItemButton onClick={handleClick} selected={props.selected} aria-label={ariaLabel} aria-live="polite">
        <ListItemIcon>{icon}</ListItemIcon>
        <ListItemText
          primary={displayName}
          primaryTypographyProps={{
            style: { overflow: "hidden", textOverflow: "ellipsis" },
          }}
        />
        {subscription.reservation?.everyone && (
          <ListItemIcon edge="end" sx={{ minWidth: "26px" }}>
            {subscription.reservation?.everyone === Permission.READ_WRITE && (
              <Tooltip title={t("prefs_reservations_table_everyone_read_write")}>
                <PermissionReadWrite size="small" />
              </Tooltip>
            )}
            {subscription.reservation?.everyone === Permission.READ_ONLY && (
              <Tooltip title={t("prefs_reservations_table_everyone_read_only")}>
                <PermissionRead size="small" />
              </Tooltip>
            )}
            {subscription.reservation?.everyone === Permission.WRITE_ONLY && (
              <Tooltip title={t("prefs_reservations_table_everyone_write_only")}>
                <PermissionWrite size="small" />
              </Tooltip>
            )}
            {subscription.reservation?.everyone === Permission.DENY_ALL && (
              <Tooltip title={t("prefs_reservations_table_everyone_deny_all")}>
                <PermissionDenyAll size="small" />
              </Tooltip>
            )}
          </ListItemIcon>
        )}
        {subscription.mutedUntil > 0 && (
          <ListItemIcon edge="end" sx={{ minWidth: "26px" }} aria-label={t("nav_button_muted")}>
            <Tooltip title={t("nav_button_muted")}>
              <NotificationsOffOutlined />
            </Tooltip>
          </ListItemIcon>
        )}
        <ListItemIcon edge="end" sx={{ minWidth: "26px" }}>
          <IconButton
            size="small"
            onMouseDown={(e) => e.stopPropagation()}
            onClick={(e) => {
              e.stopPropagation();
              setMenuAnchorEl(e.currentTarget);
            }}
          >
            <MoreVert fontSize="small" />
          </IconButton>
        </ListItemIcon>
      </ListItemButton>
      <Portal>
        <SubscriptionPopup subscription={subscription} anchor={menuAnchorEl} onClose={() => setMenuAnchorEl(null)} />
      </Portal>
    </>
  );
};

const NotificationPermissionRequired = () => {
  const { t } = useTranslation();
  const requestPermission = async () => {
    await notifier.maybeRequestPermission();
  };
  return (
    <Alert severity="warning" sx={{ paddingTop: 2 }}>
      <AlertTitle>{t("alert_notification_permission_required_title")}</AlertTitle>
      <Typography gutterBottom>{t("alert_notification_permission_required_description")}</Typography>
      <Button sx={{ float: "right" }} color="inherit" size="small" onClick={requestPermission}>
        {t("alert_notification_permission_required_button")}
      </Button>
    </Alert>
  );
};

const NotificationPermissionDeniedAlert = () => {
  const { t } = useTranslation();
  return (
    <Alert severity="warning" sx={{ paddingTop: 2 }}>
      <AlertTitle>{t("alert_notification_permission_denied_title")}</AlertTitle>
      <Typography gutterBottom>{t("alert_notification_permission_denied_description")}</Typography>
    </Alert>
  );
};

const NotificationIOSInstallRequiredAlert = () => {
  const { t } = useTranslation();
  return (
    <Alert severity="warning" sx={{ paddingTop: 2 }}>
      <AlertTitle>{t("alert_notification_ios_install_required_title")}</AlertTitle>
      <Typography gutterBottom>{t("alert_notification_ios_install_required_description")}</Typography>
    </Alert>
  );
};

const NotificationBrowserNotSupportedAlert = () => {
  const { t } = useTranslation();
  return (
    <Alert severity="warning" sx={{ paddingTop: 2 }}>
      <AlertTitle>{t("alert_not_supported_title")}</AlertTitle>
      <Typography gutterBottom>{t("alert_not_supported_description")}</Typography>
    </Alert>
  );
};

const NotificationContextNotSupportedAlert = () => {
  const { t } = useTranslation();
  return (
    <Alert severity="warning" sx={{ paddingTop: 2 }}>
      <AlertTitle>{t("alert_not_supported_title")}</AlertTitle>
      <Typography gutterBottom>
        <Trans
          i18nKey="alert_not_supported_context_description"
          components={{
            mdnLink: <Link href="https://developer.mozilla.org/en-US/docs/Web/API/notification" target="_blank" rel="noopener" />,
          }}
        />
      </Typography>
    </Alert>
  );
};

export default Navigation;
