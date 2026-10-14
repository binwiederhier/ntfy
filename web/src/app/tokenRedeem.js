import accountApi from "./AccountApi";
import session from "./Session";
import { EnrollCodeInvalidError, SubscriptionExistsError } from "./errors";
import { validTopic } from "./utils";

const tokenLabel = "Web app";

export const RedeemResult = {
  SIGNED_IN: "signed_in",
  ALREADY_SIGNED_IN: "already_signed_in",
};

export class SignedInAsOtherUserError extends Error {
  constructor(username) {
    super(`Already signed in as ${username}`);
    this.username = username;
  }
}

// parseTokenRedeemFragment reads an enrollment link's fragment: #code=<code>&user=<username>&subscribe=<t1,t2>.
// The values live in the fragment, not the query, so browsers never send them to the server.
export const parseTokenRedeemFragment = (hash) => {
  const params = new URLSearchParams(hash.replace(/^#/, ""));
  const topics = (params.get("subscribe") || "")
    .split(",")
    .map((topic) => topic.trim())
    .filter((topic) => validTopic(topic));
  return { code: params.get("code") || "", user: params.get("user") || "", topics };
};

// redeemLink signs the web app in with the link's one-time code and subscribes the account to the
// link's topics. It never replaces a session: when already signed in as the link's user (or the link
// names no user), the code is left to expire and only the topics are added; a session belonging to
// someone else is refused. The caller must reload the app afterwards, since a new session switches
// to that user's local database.
export const redeemLink = async ({ code, user, topics }) => {
  let result;
  if (session.exists()) {
    if (user && session.username() !== user) {
      throw new SignedInAsOtherUserError(session.username());
    }
    result = RedeemResult.ALREADY_SIGNED_IN;
  } else {
    if (!code) {
      throw new EnrollCodeInvalidError();
    }
    const { token, username } = await accountApi.redeemToken(code, tokenLabel);
    await session.store(username, token);
    result = RedeemResult.SIGNED_IN;
  }
  for (const topic of topics) {
    try {
      // Added to the account, not the local database, which belongs to the session the page loaded with.
      // One at a time: the server appends to the stored list, so concurrent adds could drop each other.
      // eslint-disable-next-line no-await-in-loop
      await accountApi.addSubscription(config.base_url, topic);
    } catch (e) {
      if (!(e instanceof SubscriptionExistsError)) {
        throw e;
      }
    }
  }
  return result;
};
