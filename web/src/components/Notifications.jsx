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
} from "@mui/material";
import * as React from "react";
import { useEffect, useState } from "react";
import CheckIcon from "@mui/icons-material/Check";
import CloseIcon from "@mui/icons-material/Close";
import { useLiveQuery } from "dexie-react-hooks";
import InfiniteScroll from "react-infinite-scroll-component";
import { Trans, useTranslation } from "react-i18next";
import { useOutletContext } from "react-router-dom";
import { useRemark } from "react-remark";
import styled from "@emotion/styled";
import { formatBytes, formatShortDateTime, maybeActionErrors, openUrl, shortUrl, topicShortUrl, unmatchedTags } from "../app/utils";
import { formatMessage, formatTitle, isImage } from "../app/notificationUtils";
import { LightboxBackdrop, Paragraph, VerticallyCenteredContainer } from "./styles";
import subscriptionManager from "../app/SubscriptionManager";
import priority1 from "../img/priority-1.svg";
import priority2 from "../img/priority-2.svg";
import priority4 from "../img/priority-4.svg";
import priority5 from "../img/priority-5.svg";
import logoOutline from "../img/ntfy-outline.svg";
import AttachmentIcon from "./AttachmentIcon";
import { useAutoSubscribe } from "./hooks";
import session from "../app/Session";

const priorityFiles = {
  1: priority1,
  2: priority2,
  4: priority4,
  5: priority5,
};

export const AllSubscriptions = () => {
  const { subscriptions } = useOutletContext();
  if (!subscriptions) {
    return <Loading />;
  }
  return <AllSubscriptionsList subscriptions={subscriptions} />;
};

export const SingleSubscription = () => {
  const { subscriptions, selected } = useOutletContext();
  useAutoSubscribe(subscriptions, selected);
  if (!selected) {
    return <Loading />;
  }
  return <SingleSubscriptionList subscription={selected} />;
};

const AllSubscriptionsList = (props) => {
  const { subscriptions } = props;
  const notifications = useLiveQuery(() => subscriptionManager.getAllNotifications(), []);
  if (notifications === null || notifications === undefined) {
    return <Loading />;
  }
  if (subscriptions.length === 0) {
    return <NoSubscriptions />;
  }
  if (notifications.length === 0) {
    return <NoNotificationsWithoutSubscription subscriptions={subscriptions} />;
  }
  return <NotificationList key="all" notifications={notifications} messageBar={false} />;
};

const SingleSubscriptionList = (props) => {
  const { subscription } = props;
  const notifications = useLiveQuery(() => subscriptionManager.getNotifications(subscription.id), [subscription]);
  if (notifications === null || notifications === undefined) {
    return <Loading />;
  }
  if (notifications.length === 0) {
    return <NoNotifications subscription={subscription} />;
  }
  return <NotificationList id={subscription.id} notifications={notifications} messageBar />;
};

const NotificationList = (props) => {
  const { t } = useTranslation();
  const pageSize = 20;
  const { notifications } = props;
  const [snackOpen, setSnackOpen] = useState(false);
  const [maxCount, setMaxCount] = useState(pageSize);
  const count = Math.min(notifications.length, maxCount);

  useEffect(
    () => () => {
      setMaxCount(pageSize);
      const main = document.getElementById("main");
      if (main) {
        main.scrollTo(0, 0);
      }
    },
    [props.id]
  );

  return (
    <InfiniteScroll
      dataLength={count}
      next={() => setMaxCount((prev) => prev + pageSize)}
      hasMore={count < notifications.length}
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
          {notifications.slice(0, count).map((notification) => (
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
      <Link key={i} href={parts[i]} underline="hover" target="_blank" rel="noreferrer,noopener">
        {shortUrl(parts[i])}
      </Link>
    );
  }
  return <>{parts}</>;
};

const MarkdownContainer = styled("div")`
  line-height: 1;

  h1,
  h2,
  h3,
  h4,
  h5,
  h6,
  p,
  pre,
  ul,
  ol,
  blockquote {
    margin: 0;
  }

  p {
    line-height: 1.2;
  }

  blockquote,
  pre {
    border-radius: 3px;
    background: ${(props) => (props.theme.palette.mode === "light" ? "#f5f5f5" : "#333")};
  }

  pre {
    overflow-x: scroll;
    padding: 0.9rem;
  }

  ul,
  ol,
  blockquote {
    padding-inline: 1rem;
  }

  img {
    max-width: 100%;
  }
`;

const MarkdownContent = ({ content }) => {
  const [reactContent, setMarkdownSource] = useRemark();

  useEffect(() => {
    setMarkdownSource(content);
  }, [content]);

  return <MarkdownContainer>{reactContent}</MarkdownContainer>;
};

const NotificationBody = ({ notification }) => {
  const displayAsMarkdown = notification.content_type === "text/markdown";
  const formatted = formatMessage(notification);
  if (displayAsMarkdown) {
    return <MarkdownContent content={formatted} />;
  }
  return autolink(formatted);
};

const NotificationItem = (props) => {
  const { t, i18n } = useTranslation();
  const { notification } = props;
  const { attachment } = notification;
  const date = formatShortDateTime(notification.time, i18n.language);
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
    navigator.clipboard.writeText(s);
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
        <Typography variant="body1" sx={{ whiteSpace: "pre-line" }}>
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
          {hasUserActions && <UserActions notification={notification} />}
        </CardActions>
      )}
    </Card>
  );
};

const Attachment = (props) => {
  const { t, i18n } = useTranslation();
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
        date: formatShortDateTime(attachment.expires, i18n.language),
      })
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
      <Modal open={open} onClose={() => setOpen(false)} BackdropComponent={LightboxBackdrop}>
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
      <UserAction key={action.id} notification={props.notification} action={action} />
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
  if (action.action === "broadcast") {
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
  if (action.action === "view") {
    return (
      <Tooltip title={t("notifications_actions_open_url_title", { url: action.url })}>
        <Button
          onClick={() => openUrl(action.url)}
          aria-label={t("notifications_actions_open_url_title", {
            url: action.url,
          })}
        >
          {action.label}
        </Button>
      </Tooltip>
    );
  }
  if (action.action === "http") {
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
  return null; // Others
};

const NoNotifications = (props) => {
  const { t } = useTranslation();
  const topicShortUrlResolved = topicShortUrl(props.subscription.baseUrl, props.subscription.topic);
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
          {topicShortUrlResolved}
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
  const topicShortUrlResolved = topicShortUrl(subscription.baseUrl, subscription.topic);
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
          {topicShortUrlResolved}
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
        {!session.exists() && !config.require_login && t("notifications_no_subscriptions_title")}
        {!session.exists() && config.require_login && t("notifications_no_subscriptions_login_title")}
      </Typography>
      <Paragraph>
        {!session.exists() && !config.require_login && t("notifications_no_subscriptions_description", {
          linktext: t("nav_button_subscribe"),
        })}
        {!session.exists() && config.require_login && t("notifications_no_subscriptions_login_description", {
          linktext: t("action_bar_sign_in"),
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
