import {
    fetchLinesIterator,
    maybeWithBasicAuth, maybeWithBearerAuth,
    topicShortUrl,
    topicUrl,
    topicUrlAuth,
    topicUrlJsonPoll,
    topicUrlJsonPollWithSince,
    accountSettingsUrl,
    accountTokenUrl,
    userStatsUrl, accountSubscriptionUrl, accountSubscriptionSingleUrl, accountUrl, accountPasswordUrl
} from "./utils";
import userManager from "./UserManager";

class Api {
    async poll(baseUrl, topic, since) {
        const user = await userManager.get(baseUrl);
        const shortUrl = topicShortUrl(baseUrl, topic);
        const url = (since)
            ? topicUrlJsonPollWithSince(baseUrl, topic, since)
            : topicUrlJsonPoll(baseUrl, topic);
        const messages = [];
        const headers = maybeWithBasicAuth({}, user);
        console.log(`[Api] Polling ${url}`);
        for await (let line of fetchLinesIterator(url, headers)) {
            console.log(`[Api, ${shortUrl}] Received message ${line}`);
            messages.push(JSON.parse(line));
        }
        return messages;
    }

    async publish(baseUrl, topic, message, options) {
        const user = await userManager.get(baseUrl);
        console.log(`[Api] Publishing message to ${topicUrl(baseUrl, topic)}`);
        const headers = {};
        const body = {
            topic: topic,
            message: message,
            ...options
        };
        const response = await fetch(baseUrl, {
            method: 'PUT',
            body: JSON.stringify(body),
            headers: maybeWithBasicAuth(headers, user)
        });
        if (response.status < 200 || response.status > 299) {
            throw new Error(`Unexpected response: ${response.status}`);
        }
        return response;
    }

    /**
     * Publishes to a topic using XMLHttpRequest (XHR), and returns a Promise with the active request.
     * Unfortunately, fetch() does not support a progress hook, which is why XHR has to be used.
     *
     * Firefox XHR bug:
     *    Firefox has a bug(?), which returns 0 and "" for all fields of the XHR response in the case of an error,
     *    so we cannot determine the exact error. It also sometimes complains about CORS violations, even when the
     *    correct headers are clearly set. It's quite the odd behavior.
     *
     *  There is an example, and the bug report here:
     *  - https://bugzilla.mozilla.org/show_bug.cgi?id=1733755
     *  - https://gist.github.com/binwiederhier/627f146d1959799be207ad8c17a8f345
     */
    publishXHR(url, body, headers, onProgress) {
        console.log(`[Api] Publishing message to ${url}`);
        const xhr = new XMLHttpRequest();
        const send = new Promise(function (resolve, reject) {
            xhr.open("PUT", url);
            if (body.type) {
                xhr.overrideMimeType(body.type);
            }
            for (const [key, value] of Object.entries(headers)) {
                xhr.setRequestHeader(key, value);
            }
            xhr.upload.addEventListener("progress", onProgress);
            xhr.addEventListener('readystatechange', (ev) => {
                if (xhr.readyState === 4 && xhr.status >= 200 && xhr.status <= 299) {
                    console.log(`[Api] Publish successful (HTTP ${xhr.status})`, xhr.response);
                    resolve(xhr.response);
                } else if (xhr.readyState === 4) {
                    // Firefox bug; see description above!
                    console.log(`[Api] Publish failed (HTTP ${xhr.status})`, xhr.responseText);
                    let errorText;
                    try {
                        const error = JSON.parse(xhr.responseText);
                        if (error.code && error.error) {
                            errorText = `Error ${error.code}: ${error.error}`;
                        }
                    } catch (e) {
                        // Nothing
                    }
                    xhr.abort();
                    reject(errorText ?? "An error occurred");
                }
            })
            xhr.send(body);
        });
        send.abort = () => {
            console.log(`[Api] Publish aborted by user`);
            xhr.abort();
        }
        return send;
    }

    async topicAuth(baseUrl, topic, user) {
        const url = topicUrlAuth(baseUrl, topic);
        console.log(`[Api] Checking auth for ${url}`);
        const response = await fetch(url, {
            headers: maybeWithBasicAuth({}, user)
        });
        if (response.status >= 200 && response.status <= 299) {
            return true;
        } else if (!user && response.status === 404) {
            return true; // Special case: Anonymous login to old servers return 404 since /<topic>/auth doesn't exist
        } else if (response.status === 401 || response.status === 403) { // See server/server.go
            return false;
        }
        throw new Error(`Unexpected server response ${response.status}`);
    }

    async login(baseUrl, user) {
        const url = accountTokenUrl(baseUrl);
        console.log(`[Api] Checking auth for ${url}`);
        const response = await fetch(url, {
            headers: maybeWithBasicAuth({}, user)
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
        const json = await response.json();
        if (!json.token) {
            throw new Error(`Unexpected server response: Cannot find token`);
        }
        return json.token;
    }

    async logout(baseUrl, token) {
        const url = accountTokenUrl(baseUrl);
        console.log(`[Api] Logging out from ${url} using token ${token}`);
        const response = await fetch(url, {
            method: "DELETE",
            headers: maybeWithBearerAuth({}, token)
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async createAccount(baseUrl, username, password) {
        const url = accountUrl(baseUrl);
        const body = JSON.stringify({
            username: username,
            password: password
        });
        console.log(`[Api] Creating user account ${url}`);
        const response = await fetch(url, {
            method: "POST",
            body: body
        });
        if (response.status === 409) {
            throw new UsernameTakenError(username);
        } else if (response.status === 429) {
            throw new AccountCreateLimitReachedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async getAccount(baseUrl, token) {
        const url = accountUrl(baseUrl);
        console.log(`[Api] Fetching user account ${url}`);
        const response = await fetch(url, {
            headers: maybeWithBearerAuth({}, token)
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
        const account = await response.json();
        console.log(`[Api] Account`, account);
        return account;
    }

    async deleteAccount(baseUrl, token) {
        const url = accountUrl(baseUrl);
        console.log(`[Api] Deleting user account ${url}`);
        const response = await fetch(url, {
            method: "DELETE",
            headers: maybeWithBearerAuth({}, token)
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async changePassword(baseUrl, token, password) {
        const url = accountPasswordUrl(baseUrl);
        console.log(`[Api] Changing account password ${url}`);
        const response = await fetch(url, {
            method: "POST",
            headers: maybeWithBearerAuth({}, token),
            body: JSON.stringify({
                password: password
            })
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async updateAccountSettings(baseUrl, token, payload) {
        const url = accountSettingsUrl(baseUrl);
        const body = JSON.stringify(payload);
        console.log(`[Api] Updating user account ${url}: ${body}`);
        const response = await fetch(url, {
            method: "POST",
            headers: maybeWithBearerAuth({}, token),
            body: body
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async addAccountSubscription(baseUrl, token, payload) {
        const url = accountSubscriptionUrl(baseUrl);
        const body = JSON.stringify(payload);
        console.log(`[Api] Adding user subscription ${url}: ${body}`);
        const response = await fetch(url, {
            method: "POST",
            headers: maybeWithBearerAuth({}, token),
            body: body
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
        const subscription = await response.json();
        console.log(`[Api] Subscription`, subscription);
        return subscription;
    }

    async deleteAccountSubscription(baseUrl, token, remoteId) {
        const url = accountSubscriptionSingleUrl(baseUrl, remoteId);
        console.log(`[Api] Removing user subscription ${url}`);
        const response = await fetch(url, {
            method: "DELETE",
            headers: maybeWithBearerAuth({}, token)
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }
}

export class UsernameTakenError extends Error {
    constructor(username) {
        super("Username taken");
        this.username = username;
    }
}

export class AccountCreateLimitReachedError extends Error {
    constructor() {
        super("Account creation limit reached");
    }
}

export class UnauthorizedError extends Error {
    constructor() {
        super("Unauthorized");
    }
}

const api = new Api();
export default api;
