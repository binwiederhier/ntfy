import { describe, expect, it } from "vitest";
import emojisMapped from "./emojisMapped";
import { formatTitle, formatMessage, isImage, notificationTag, toNotificationParams, messageWithSequenceId } from "./notificationUtils";

// Pick a real alias/emoji pair so we don't hardcode a specific glyph that could change upstream.
const [emojiAlias, emojiChar] = Object.entries(emojisMapped)[0];

describe("formatTitle", () => {
  it("returns the bare title when there are no emoji tags", () => {
    expect(formatTitle({ title: "Hello", tags: ["not-an-emoji"] })).toBe("Hello");
    expect(formatTitle({ title: "Hello" })).toBe("Hello");
  });

  it("prepends mapped emoji for emoji tags", () => {
    expect(formatTitle({ title: "Hello", tags: [emojiAlias] })).toBe(`${emojiChar} Hello`);
  });
});

describe("formatMessage", () => {
  it("returns the message untouched when the notification has a title", () => {
    expect(formatMessage({ title: "T", message: "Body", tags: [emojiAlias] })).toBe("Body");
  });

  it("prepends emoji to the message when there is no title", () => {
    expect(formatMessage({ message: "Body", tags: [emojiAlias] })).toBe(`${emojiChar} Body`);
  });

  it("falls back to an empty string when message is missing", () => {
    expect(formatMessage({})).toBe("");
  });
});

describe("isImage", () => {
  it("trusts an explicit image MIME type", () => {
    expect(isImage({ type: "image/png", name: "whatever.txt" })).toBe(true);
    expect(isImage({ type: "application/pdf", name: "x.png" })).toBe(false);
  });

  it("falls back to the file extension when there is no type", () => {
    expect(isImage({ name: "photo.JPEG" })).toBeTruthy();
    expect(isImage({ url: "https://ntfy.sh/file/x.webp" })).toBeTruthy();
    expect(isImage({ name: "notes.txt" })).toBeFalsy();
  });

  it("returns false for a missing attachment", () => {
    expect(isImage(undefined)).toBe(false);
  });
});

describe("notificationTag", () => {
  it("scopes the tag by baseUrl, topic and sequence id", () => {
    expect(notificationTag("https://ntfy.sh", "mytopic", 42)).toBe("https://ntfy.sh/mytopic/42");
  });
});

describe("toNotificationParams", () => {
  const baseMessage = {
    id: "msg-1",
    time: 1700000000,
    title: "Title",
    message: "Body",
    actions: [
      { action: "view", label: "Open" },
      { action: "http", label: "Send" },
      { action: "broadcast", label: "Cast" },
    ],
  };

  it("builds the [title, options] tuple consumed by the Notifications API", () => {
    const [title, options] = toNotificationParams({
      message: baseMessage,
      defaultTitle: "fallback",
      topicRoute: "/mytopic",
      baseUrl: "https://ntfy.sh",
      topic: "mytopic",
    });

    expect(title).toBe("Title");
    expect(options.body).toBe("Body");
    expect(options.tag).toBe("https://ntfy.sh/mytopic/msg-1");
    expect(options.timestamp).toBe(baseMessage.time * 1000);
    expect(options.data.subscriptionId).toBe("https://ntfy.sh/mytopic");
    expect(options.data.topicRoute).toBe("/mytopic");
  });

  it("keeps only view/http actions and maps them to {action,title}", () => {
    const [, options] = toNotificationParams({
      message: baseMessage,
      defaultTitle: "fallback",
      topicRoute: "/mytopic",
      baseUrl: "https://ntfy.sh",
      topic: "mytopic",
    });
    expect(options.actions).toEqual([
      { action: "Open", title: "Open" },
      { action: "Send", title: "Send" },
    ]);
  });

  it("uses the default title when the message has none", () => {
    const [title] = toNotificationParams({
      message: { id: "x", time: 1, message: "Body" },
      defaultTitle: "fallback",
      topicRoute: "/mytopic",
      baseUrl: "https://ntfy.sh",
      topic: "mytopic",
    });
    expect(title).toBe("fallback");
  });

  it("prefers sequence_id over id for the notification tag", () => {
    const [, options] = toNotificationParams({
      message: { ...baseMessage, sequence_id: 99 },
      defaultTitle: "fallback",
      topicRoute: "/mytopic",
      baseUrl: "https://ntfy.sh",
      topic: "mytopic",
    });
    expect(options.tag).toBe("https://ntfy.sh/mytopic/99");
  });

  it("uses the first attachment image from attachments", () => {
    const [, options] = toNotificationParams({
      message: {
        ...baseMessage,
        attachments: [
          { name: "image.png", url: "https://ntfy.sh/file/image.png" },
          { name: "other.png", url: "https://ntfy.sh/file/other.png" },
        ],
      },
      defaultTitle: "fallback",
      topicRoute: "/mytopic",
      baseUrl: "https://ntfy.sh",
      topic: "mytopic",
    });
    expect(options.image).toBe("https://ntfy.sh/file/image.png");
  });
});

describe("messageWithSequenceId", () => {
  it("derives sequenceId from sequence_id when absent", () => {
    expect(messageWithSequenceId({ id: "a", sequence_id: 7 }).sequenceId).toBe(7);
  });

  it("falls back to id when sequence_id is missing", () => {
    expect(messageWithSequenceId({ id: "a" }).sequenceId).toBe("a");
  });

  it("returns the message unchanged when sequenceId already set", () => {
    const m = { id: "a", sequenceId: 1 };
    expect(messageWithSequenceId(m)).toBe(m);
  });
});
