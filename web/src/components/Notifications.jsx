import {
  Container,
  ButtonBase,
  CardActions,
  CardContent,
  CircularProgress,
  Fade,
  Link,
  Modal,
  Snackbar,
  Stack,
  Tooltip,
  Card,
  Typography,
  IconButton,
  Box,
  Button,
  InputAdornment,
  TextField,
} from "@mui/material";
import * as React from "react";
import { Suspense, lazy, useEffect, useMemo, useRef, useState } from "react";
import CheckIcon from "@mui/icons-material/Check";
import CloseIcon from "@mui/icons-material/Close";
import SearchIcon from "@mui/icons-material/Search";
import InfiniteScroll from "react-infinite-scroll-component";
import { Trans, useTranslation } from "react-i18next";
import { useOutletContext } from "react-router-dom";
import { copyToClipboard, formatBytes, formatDateTime, maybeActionErrors, openUrl, shortUrl, topicUrl, unmatchedTags } from "../app/utils";
import { ACTION_BROADCAST, ACTION_COPY, ACTION_HTTP, ACTION_VIEW } from "../app/actions";
import { formatMessage, formatTitle, isImage } from "../app/notificationUtils";
import { LightboxBackdrop, Paragraph, VerticallyCenteredContainer } from "./styles";
import subscriptionManager from "../app/SubscriptionManager";
import notifier from "../app/Notifier";
import priority1 from "../img/priority-1.svg";
import priority2 from "../img/priority-2.svg";
import priority4 from "../img/priority-4.svg";
import priority5 from "../img/priority-5.svg";
import logoOutline from "../img/ntfy-outline.svg";
import AttachmentIcon from "./AttachmentIcon";
import { useAutoSubscribe } from "./hooks";
import { usePrefCache } from "./PrefCache";

const priorityFiles = {
  1: priority1,
  2: priority2,
  4: priority4,
  5: priority5,
};

export const AllSubscriptions = () => {
  // allNotifications is preloaded in Layout, so this view has its data on mount (no empty frame on switch).
  const { subscriptions, allNotifications } = useOutletContext();
  if (!subscriptions || allNotifications === null || allNotifications === undefined) {
    return <DeferredLoading />;
  }
  return <AllSubscriptionsList subscriptions={subscriptions} notifications={allNotifications} />;
};

export const SingleSubscription = () => {
  const { subscriptions, selected, allNotifications } = useOutletContext();
  useAutoSubscribe(subscriptions, selected);
  if (!selected || allNotifications === null || allNotifications === undefined) {
    return <DeferredLoading />;
  }
  return <SingleSubscriptionList subscription={selected} allNotifications={allNotifications} />;
};

const AllSubscriptionsList = (props) => {
  const { subscriptions, notifications } = props;
  if (subscriptions.length === 0) {
    return <NoSubscriptions />;
  }
  if (notifications.length === 0) {
    return <NoNotificationsWithoutSubscription subscriptions={subscriptions} />;
  }
  return <NotificationList key="all" notifications={notifications} messageBar={false} />;
};

const SingleSubscriptionList = (props) => {
  const { subscription, allNotifications } = props;
  // Filter the preloaded allNotifications instead of a per-topic query (getNotifications(id) ==
  // getAllNotifications() filtered by id), so topic switches are instant.
  const notifications = useMemo(
    () => allNotifications.filter((notification) => notification.subscriptionId === subscription.id),
    [allNotifications, subscription.id],
  );
  if (notifications.length === 0) {
    return <NoNotifications subscription={subscription} />;
  }
  return <NotificationList key={subscription.id} id={subscription.id} notifications={notifications} messageBar />;
};

const NotificationList = (props) => {
  const { t } = useTranslation();
  const pageSize = 20;
  const { notifications } = props;
  const [snackOpen, setSnackOpen] = useState(false);
  const [maxCount, setMaxCount] = useState(pageSize);
  const [query, setQuery] = useState("");
  const searchInput = useRef();
  const search = query.trim().toLowerCase();
  const filteredNotifications = useMemo(
    () =>
      search
        ? notifications.filter((notification) =>
            [notification.title ?? "", notification.message ?? "", ...(notification.tags ?? [])].some((text) =>
              text.toLowerCase().includes(search),
            ),
          )
        : notifications,
    [notifications, search],
  );
  const count = Math.min(filteredNotifications.length, maxCount);

  const updateQuery = (value) => {
    setQuery(value);
    setMaxCount(pageSize);
    document.getElementById("main")?.scrollTo(0, 0);
  };

  const clearSearch = () => {
    updateQuery("");
    searchInput.current?.focus();
  };

  useEffect(
    () => () => {
      setMaxCount(pageSize);
      const main = document.getElementById("main");
      if (main) {
        main.scrollTo(0, 0);
      }
    },
    [props.id],
  );

  return (
    <>
      <Container maxWidth="md" sx={{ marginTop: 3 }}>
        <TextField
          fullWidth
          size="small"
          type="search"
          sx={{ "& input::-webkit-search-cancel-button": { display: "none" } }}
          label={t("notifications_search")}
          inputRef={searchInput}
          value={query}
          onChange={(event) => updateQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              clearSearch();
            }
          }}
          slotProps={{
            input: {
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon />
                </InputAdornment>
              ),
              endAdornment: query && (
                <InputAdornment position="end">
                  <IconButton size="small" onClick={clearSearch} aria-label={t("notifications_search_clear")}>
                    <CloseIcon fontSize="small" />
                  </IconButton>
                </InputAdornment>
              ),
            },
          }}
        />
      </Container>
      {filteredNotifications.length === 0 && (
        <Container maxWidth="md" sx={{ marginTop: 3 }}>
          <Typography role="status" color="text.secondary">
            {t("notifications_search_no_results")}
          </Typography>
        </Container>
      )}
      <InfiniteScroll
        key={search}
        dataLength={count}
        next={() => setMaxCount((prev) => prev + pageSize)}
        hasMore={count < filteredNotifications.length}
        loader={<>Loading ...</>}
        scrollThreshold={0.7}
        scrollableTarget="main"
      >
        <Container
          maxWidth="md"
          role="list"
          aria-label={t("notifications_list")}
          sx={{
            marginTop: 3,
            marginBottom: props.messageBar ? "100px" : 3, // Hack to avoid hiding notifications behind the message bar
          }}
        >
          <Stack spacing={3}>
            {filteredNotifications.slice(0, count).map((notification) => (
              <NotificationItem key={notification.id} notification={notification} onShowSnack={() => setSnackOpen(true)} />
            ))}
            <Snackbar
              open={snackOpen}
              autoHideDuration={3000}
              onClose={() => setSnackOpen(false)}
              message={t("notifications_copied_to_clipboard")}
            />
          </Stack>
        </Container>
      </InfiniteScroll>
    </>
  );
};

/**
 * Replace links with <Link/> components; this is a combination of the genius function
 * in [1] and the regex in [2].
 *
 * [1] https://github.com/facebook/react/issues/3386#issuecomment-78605760
 * [2] https://github.com/bryanwoods/autolink-js/blob/master/autolink.js#L9
 */
const autolink = (s) => {
  const parts = s.split(/(\bhttps?:\/\/[-A-Z0-9+\u0026\u2019@#/%?=()~_|!:,.;]*[-A-Z0-9+\u0026@#/%=~()_|]\b)/gi);
  for (let i = 1; i < parts.length; i += 2) {
    parts[i] = (
      <Link key={i} href={parts[i]} underline="hover" target="_blank" rel="noreferrer">
        {shortUrl(parts[i])}
      </Link>
    );
  }
  return <>{parts}</>;
};

// Loaded lazily so the heavy react-remark/unified markdown stack is only fetched when a
// text/markdown notification is actually rendered (see MarkdownContent.jsx).
const MarkdownContent = lazy(() => import("./MarkdownContent"));

const NotificationBody = ({ notification }) => {
  const displayAsMarkdown = notification.content_type === "text/markdown";
  const formatted = formatMessage(notification);
  if (displayAsMarkdown) {
    return (
      <Suspense fallback={null}>
        <MarkdownContent content={formatted} />
      </Suspense>
    );
  }
  return autolink(formatted);
};

const NotificationItem = (props) => {
  const { t } = useTranslation();
  const { dateFormat, timeFormat } = usePrefCache();
  const { notification } = props;
  const { attachment } = notification;
  const date = formatDateTime(notification.time, dateFormat, timeFormat);
  const otherTags = unmatchedTags(notification.tags);
  const tags = otherTags.length > 0 ? otherTags.join(", ") : null;
  const handleDelete = async () => {
    console.log(`[Notifications] Deleting notification ${notification.id}`);
    await subscriptionManager.deleteNotification(notification.id);
  };
  const handleMarkRead = async () => {
    console.log(`[Notifications] Marking notification ${notification.id} as read`);
    await subscriptionManager.markNotificationRead(notification.id);
  };
  const handleCopy = (s) => {
    copyToClipboard(s);
    props.onShowSnack();
  };
  const expired = attachment && attachment.expires && attachment.expires < Date.now() / 1000;
  const hasAttachmentActions = attachment && !expired;
  const hasClickAction = notification.click;
  const hasUserActions = notification.actions && notification.actions.length > 0;
  const showActions = hasAttachmentActions || hasClickAction || hasUserActions;

  return (
    <Card sx={{ padding: 1 }} role="listitem" aria-label={t("notifications_list_item")}>
      <CardContent>
        <Tooltip title={t("notifications_delete")} enterDelay={500}>
          <IconButton onClick={handleDelete} sx={{ float: "right", marginRight: -1, marginTop: -1 }} aria-label={t("notifications_delete")}>
            <CloseIcon />
          </IconButton>
        </Tooltip>
        {notification.new === 1 && (
          <Tooltip title={t("notifications_mark_read")} enterDelay={500}>
            <IconButton
              onClick={handleMarkRead}
              sx={{ float: "right", marginRight: -0.5, marginTop: -1 }}
              aria-label={t("notifications_mark_read")}
            >
              <CheckIcon />
            </IconButton>
          </Tooltip>
        )}
        <Typography sx={{ fontSize: 14 }} color="text.secondary">
          {date}
          {[1, 2, 4, 5].includes(notification.priority) && (
            <img
              src={priorityFiles[notification.priority]}
              alt={t("notifications_priority_x", {
                priority: notification.priority,
              })}
              style={{ verticalAlign: "bottom" }}
            />
          )}
          {notification.new === 1 && (
            <svg
              style={{ width: "8px", height: "8px", marginLeft: "4px" }}
              viewBox="0 0 100 100"
              xmlns="http://www.w3.org/2000/svg"
              aria-label={t("notifications_new_indicator")}
            >
              <circle cx="50" cy="50" r="50" fill="#338574" />
            </svg>
          )}
        </Typography>
        {notification.title && (
          <Typography variant="h5" component="div" role="rowheader">
            {formatTitle(notification)}
          </Typography>
        )}
        <Typography variant="body1" sx={{ whiteSpace: "pre-line", overflowX: "auto" }}>
          <NotificationBody notification={notification} />
          {maybeActionErrors(notification)}
        </Typography>
        {attachment && <Attachment attachment={attachment} />}
        {tags && (
          <Typography sx={{ fontSize: 14 }} color="text.secondary">
            {t("notifications_tags")}: {tags}
          </Typography>
        )}
      </CardContent>
      {showActions && (
        <CardActions sx={{ paddingTop: 0 }}>
          {hasAttachmentActions && (
            <>
              <Tooltip title={t("notifications_attachment_copy_url_title")}>
                <Button onClick={() => handleCopy(attachment.url)}>{t("notifications_attachment_copy_url_button")}</Button>
              </Tooltip>
              <Tooltip
                title={t("notifications_attachment_open_title", {
                  url: attachment.url,
                })}
              >
                <Button onClick={() => openUrl(attachment.url)}>{t("notifications_attachment_open_button")}</Button>
              </Tooltip>
            </>
          )}
          {hasClickAction && (
            <>
              <Tooltip title={t("notifications_click_copy_url_title")}>
                <Button onClick={() => handleCopy(notification.click)}>{t("notifications_click_copy_url_button")}</Button>
              </Tooltip>
              <Tooltip
                title={t("notifications_actions_open_url_title", {
                  url: notification.click,
                })}
              >
                <Button onClick={() => openUrl(notification.click)}>{t("notifications_click_open_button")}</Button>
              </Tooltip>
            </>
          )}
          {hasUserActions && <UserActions notification={notification} onShowSnack={props.onShowSnack} />}
        </CardActions>
      )}
    </Card>
  );
};

const Attachment = (props) => {
  const { t } = useTranslation();
  const { dateFormat, timeFormat } = usePrefCache();
  const { attachment } = props;
  const expired = attachment.expires && attachment.expires < Date.now() / 1000;
  const expires = attachment.expires && attachment.expires > Date.now() / 1000;
  const displayableImage = !expired && isImage(attachment);

  // Unexpired image
  if (displayableImage) {
    return <Image attachment={attachment} />;
  }

  // Anything else: Show box
  const infos = [];
  if (attachment.size) {
    infos.push(formatBytes(attachment.size));
  }
  if (expires) {
    infos.push(
      t("notifications_attachment_link_expires", {
        date: formatDateTime(attachment.expires, dateFormat, timeFormat),
      }),
    );
  }
  if (expired) {
    infos.push(t("notifications_attachment_link_expired"));
  }
  const maybeInfoText =
    infos.length > 0 ? (
      <>
        <br />
        {infos.join(", ")}
      </>
    ) : null;

  // If expired, just show infos without click target
  if (expired) {
    return (
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          marginTop: 2,
          padding: 1,
          borderRadius: "4px",
        }}
      >
        <AttachmentIcon type={attachment.type} />
        <Typography variant="body2" sx={{ marginLeft: 1, textAlign: "left", color: "text.primary" }}>
          <b>{attachment.name}</b>
          {maybeInfoText}
        </Typography>
      </Box>
    );
  }

  // Not expired
  return (
    <ButtonBase
      sx={{
        marginTop: 2,
      }}
    >
      <Link
        href={attachment.url}
        target="_blank"
        rel="noopener"
        underline="none"
        sx={{
          display: "flex",
          alignItems: "center",
          padding: 1,
          borderRadius: "4px",
          "&:hover": {
            backgroundColor: "rgba(0, 0, 0, 0.05)",
          },
        }}
      >
        <AttachmentIcon type={attachment.type} />
        <Typography variant="body2" sx={{ marginLeft: 1, textAlign: "left", color: "text.primary" }}>
          <b>{attachment.name}</b>
          {maybeInfoText}
        </Typography>
      </Link>
    </ButtonBase>
  );
};

const Image = (props) => {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Box
        component="img"
        src={props.attachment.url}
        loading="lazy"
        alt={t("notifications_attachment_image")}
        onClick={() => setOpen(true)}
        sx={{
          marginTop: 2,
          borderRadius: "4px",
          boxShadow: 2,
          width: 1,
          maxHeight: "400px",
          objectFit: "cover",
          cursor: "pointer",
        }}
      />
      <Modal open={open} onClose={() => setOpen(false)} slots={{ backdrop: LightboxBackdrop }}>
        <Fade in={open}>
          <Box
            component="img"
            src={props.attachment.url}
            alt={t("notifications_attachment_image")}
            loading="lazy"
            sx={{
              maxWidth: 1,
              maxHeight: 1,
              position: "absolute",
              top: "50%",
              left: "50%",
              transform: "translate(-50%, -50%)",
              padding: 4,
            }}
          />
        </Fade>
      </Modal>
    </>
  );
};

const UserActions = (props) => (
  <>
    {props.notification.actions.map((action) => (
      <UserAction key={action.id} notification={props.notification} action={action} onShowSnack={props.onShowSnack} />
    ))}
  </>
);

const ACTION_PROGRESS_ONGOING = 1;
const ACTION_PROGRESS_SUCCESS = 2;
const ACTION_PROGRESS_FAILED = 3;

const ACTION_LABEL_SUFFIX = {
  [ACTION_PROGRESS_ONGOING]: " …",
  [ACTION_PROGRESS_SUCCESS]: " ✔",
  [ACTION_PROGRESS_FAILED]: " ❌",
};

const updateActionStatus = (notification, action, progress, error) => {
  subscriptionManager.updateNotification({
    ...notification,
    actions: notification.actions.map((a) => (a.id === action.id ? { ...a, progress, error } : a)),
  });
};

const clearNotification = async (notification) => {
  console.log(`[Notifications] Clearing notification ${notification.id}`);
  const subscription = await subscriptionManager.get(notification.subscriptionId);
  if (subscription) {
    await notifier.cancel(subscription, notification);
  }
  await subscriptionManager.markNotificationRead(notification.id);
};

const performHttpAction = async (notification, action) => {
  console.log(`[Notifications] Performing HTTP user action`, action);
  try {
    updateActionStatus(notification, action, ACTION_PROGRESS_ONGOING, null);
    const response = await fetch(action.url, {
      method: action.method ?? "POST",
      headers: action.headers ?? {},
      // This must not null-coalesce to a non nullish value. Otherwise, the fetch API
      // will reject it for "having a body"
      body: action.body,
    });
    console.log(`[Notifications] HTTP user action response`, response);
    const success = response.status >= 200 && response.status <= 299;
    if (success) {
      updateActionStatus(notification, action, ACTION_PROGRESS_SUCCESS, null);
      if (action.clear) {
        await clearNotification(notification);
      }
    } else {
      updateActionStatus(notification, action, ACTION_PROGRESS_FAILED, `${action.label}: Unexpected response HTTP ${response.status}`);
    }
  } catch (e) {
    console.log(`[Notifications] HTTP action failed`, e);
    updateActionStatus(notification, action, ACTION_PROGRESS_FAILED, `${action.label}: ${e} Check developer console for details.`);
  }
};

const UserAction = (props) => {
  const { t } = useTranslation();
  const { notification } = props;
  const { action } = props;
  if (action.action === ACTION_BROADCAST) {
    return (
      <Tooltip title={t("notifications_actions_not_supported")}>
        <span>
          <Button disabled aria-label={t("notifications_actions_not_supported")}>
            {action.label}
          </Button>
        </span>
      </Tooltip>
    );
  }
  if (action.action === ACTION_VIEW) {
    const handleClick = () => {
      openUrl(action.url);
      if (action.clear) {
        clearNotification(notification);
      }
    };
    return (
      <Tooltip title={t("notifications_actions_open_url_title", { url: action.url })}>
        <Button
          onClick={handleClick}
          aria-label={t("notifications_actions_open_url_title", {
            url: action.url,
          })}
        >
          {action.label}
        </Button>
      </Tooltip>
    );
  }
  if (action.action === ACTION_HTTP) {
    const method = action.method ?? "POST";
    const label = action.label + (ACTION_LABEL_SUFFIX[action.progress ?? 0] ?? "");
    return (
      <Tooltip
        title={t("notifications_actions_http_request_title", {
          method,
          url: action.url,
        })}
      >
        <Button
          onClick={() => performHttpAction(notification, action)}
          aria-label={t("notifications_actions_http_request_title", {
            method,
            url: action.url,
          })}
        >
          {label}
        </Button>
      </Tooltip>
    );
  }
  if (action.action === ACTION_COPY) {
    const handleClick = async () => {
      await copyToClipboard(action.value);
      props.onShowSnack();
      if (action.clear) {
        await clearNotification(notification);
      }
    };
    return (
      <Tooltip title={t("common_copy_to_clipboard")}>
        <Button onClick={handleClick} aria-label={t("common_copy_to_clipboard")}>
          {action.label}
        </Button>
      </Tooltip>
    );
  }
  return null; // Others
};

const NoNotifications = (props) => {
  const { t } = useTranslation();
  const topicUrlResolved = topicUrl(props.subscription.baseUrl, props.subscription.topic);
  return (
    <VerticallyCenteredContainer maxWidth="xs">
      <Typography variant="h5" align="center" sx={{ paddingBottom: 1 }}>
        <img src={logoOutline} height="64" width="64" alt={t("action_bar_logo_alt")} />
        <br />
        {t("notifications_none_for_topic_title")}
      </Typography>
      <Paragraph>{t("notifications_none_for_topic_description")}</Paragraph>
      <Paragraph>
        {t("notifications_example")}:<br />
        <tt>
          {'$ curl -d "Hi" '}
          {topicUrlResolved}
        </tt>
      </Paragraph>
      <Paragraph>
        <ForMoreDetails />
      </Paragraph>
    </VerticallyCenteredContainer>
  );
};

const NoNotificationsWithoutSubscription = (props) => {
  const { t } = useTranslation();
  const subscription = props.subscriptions[0];
  const topicUrlResolved = topicUrl(subscription.baseUrl, subscription.topic);
  return (
    <VerticallyCenteredContainer maxWidth="xs">
      <Typography variant="h5" align="center" sx={{ paddingBottom: 1 }}>
        <img src={logoOutline} height="64" width="64" alt={t("action_bar_logo_alt")} />
        <br />
        {t("notifications_none_for_any_title")}
      </Typography>
      <Paragraph>{t("notifications_none_for_any_description")}</Paragraph>
      <Paragraph>
        {t("notifications_example")}:<br />
        <tt>
          {'$ curl -d "Hi" '}
          {topicUrlResolved}
        </tt>
      </Paragraph>
      <Paragraph>
        <ForMoreDetails />
      </Paragraph>
    </VerticallyCenteredContainer>
  );
};

const NoSubscriptions = () => {
  const { t } = useTranslation();
  return (
    <VerticallyCenteredContainer maxWidth="xs">
      <Typography variant="h5" align="center" sx={{ paddingBottom: 1 }}>
        <img src={logoOutline} height="64" width="64" alt={t("action_bar_logo_alt")} />
        <br />
        {t("notifications_no_subscriptions_title")}
      </Typography>
      <Paragraph>
        {t("notifications_no_subscriptions_description", {
          linktext: t("nav_button_subscribe"),
        })}
      </Paragraph>
      <Paragraph>
        <ForMoreDetails />
      </Paragraph>
    </VerticallyCenteredContainer>
  );
};

const ForMoreDetails = () => (
  <Trans
    i18nKey="notifications_more_details"
    components={{
      websiteLink: <Link href="https://ntfy.sh" target="_blank" rel="noopener" />,
      docsLink: <Link href="https://ntfy.sh/docs" target="_blank" rel="noopener" />,
    }}
  />
);

const Loading = () => {
  const { t } = useTranslation();
  return (
    <VerticallyCenteredContainer>
      <Typography variant="h5" color="text.secondary" align="center" sx={{ paddingBottom: 1 }}>
        <CircularProgress disableShrink sx={{ marginBottom: 1 }} />
        <br />
        {t("notifications_loading")}
      </Typography>
    </VerticallyCenteredContainer>
  );
};

// Render nothing until a load takes at least `delayMs`, so the centered spinner only shows on
// genuinely slow loads -- normal sub-frame IndexedDB reads don't flash it on every remount.
const DeferredLoading = ({ delayMs = 250 }) => {
  const [show, setShow] = useState(false);
  useEffect(() => {
    const timer = setTimeout(() => setShow(true), delayMs);
    return () => clearTimeout(timer);
  }, [delayMs]);
  return show ? <Loading /> : null;
};
