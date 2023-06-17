// This is a separate file since the other utils import `config.js`, which depends on `window`
// and cannot be used in the service worker

import emojisMapped from "./emojisMapped";

const toEmojis = (tags) => {
  if (!tags) return [];
  return tags.filter((tag) => tag in emojisMapped).map((tag) => emojisMapped[tag]);
};

export const formatTitle = (m) => {
  const emojiList = toEmojis(m.tags);
  if (emojiList.length > 0) {
    return `${emojiList.join(" ")} ${m.title}`;
  }
  return m.title;
};

const formatTitleWithDefault = (m, fallback) => {
  if (m.title) {
    return formatTitle(m);
  }
  return fallback;
};

export const formatMessage = (m) => {
  if (m.title) {
    return m.message;
  }
  const emojiList = toEmojis(m.tags);
  if (emojiList.length > 0) {
    return `${emojiList.join(" ")} ${m.message}`;
  }
  return m.message;
};

const imageRegex = /\.(png|jpe?g|gif|webp)$/i;
const isImage = (attachment) => {
  if (!attachment) return false;

  // if there's a type, only take that into account
  if (attachment.type) {
    return attachment.type.startsWith("image/");
  }

  // otherwise, check the extension
  return attachment.name?.match(imageRegex) || attachment.url?.match(imageRegex);
};

export const icon = "/static/images/ntfy.png";
export const badge = "/static/images/mask-icon.svg";

export const toNotificationParams = ({ subscriptionId, message, defaultTitle, topicRoute }) => {
  const image = isImage(message.attachment) ? message.attachment.url : undefined;

  // https://developer.mozilla.org/en-US/docs/Web/API/Notifications_API
  return [
    formatTitleWithDefault(message, defaultTitle),
    {
      body: formatMessage(message),
      badge,
      icon,
      image,
      timestamp: message.time * 1_000,
      tag: subscriptionId,
      renotify: true,
      silent: false,
      // This is used by the notification onclick event
      data: {
        message,
        topicRoute,
      },
      actions: message.actions
        ?.filter(({ action }) => action === "view" || action === "http")
        .map(({ label }) => ({
          action: label,
          title: label,
        })),
    },
  ];
};
