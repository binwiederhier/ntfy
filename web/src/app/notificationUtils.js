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

export const formatTitleWithDefault = (m, fallback) => {
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
