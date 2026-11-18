const sharePrefix = "#share?";

export const getShareFragment = (hash) => (hash.startsWith(sharePrefix) ? hash : "");

export const readShareDraft = (hash) => {
  if (!getShareFragment(hash)) {
    return null;
  }
  const params = new URLSearchParams(hash.slice(sharePrefix.length));
  const title = params.get("title")?.trim() ?? "";
  const text = params.get("text") ?? "";
  const url = params.get("url")?.trim() ?? "";
  // Some share sources include the link in both fields.
  const message = url && !text.split(/\s+/).includes(url) ? [text, url].filter((part) => part.trim()).join("\n\n") : text;
  return title || message.trim() ? { title, message } : null;
};
