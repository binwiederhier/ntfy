import {
    accountPasswordUrl,
    accountSettingsUrl,
    accountSubscriptionSingleUrl,
    accountSubscriptionUrl,
    accountTokenUrl,
    accountUrl,
    fetchLinesIterator,
    maybeWithBasicAuth,
    maybeWithBearerAuth,
    topicShortUrl,
    topicUrl,
    topicUrlAuth,
    topicUrlJsonPoll,
    topicUrlJsonPollWithSince
} from "./utils";
import userManager from "./UserManager";
import session from "./Session";

class AccountApi {
    async login(user) {
        const url = accountTokenUrl(config.baseUrl);
        console.log(`[Api] Checking auth for ${url}`);
        const response = await fetch(url, {
            method: "POST",
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

    async logout(token) {
        const url = accountTokenUrl(config.baseUrl);
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

    async create(username, password) {
        const url = accountUrl(config.baseUrl);
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

    async get() {
        const url = accountUrl(config.baseUrl);
        console.log(`[Api] Fetching user account ${url}`);
        const response = await fetch(url, {
            headers: maybeWithBearerAuth({}, session.token())
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

    async delete() {
        const url = accountUrl(config.baseUrl);
        console.log(`[Api] Deleting user account ${url}`);
        const response = await fetch(url, {
            method: "DELETE",
            headers: maybeWithBearerAuth({}, session.token())
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async changePassword(newPassword) {
        const url = accountPasswordUrl(config.baseUrl);
        console.log(`[Api] Changing account password ${url}`);
        const response = await fetch(url, {
            method: "POST",
            headers: maybeWithBearerAuth({}, session.token()),
            body: JSON.stringify({
                password: newPassword
            })
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async extendToken() {
        const url = accountTokenUrl(config.baseUrl);
        console.log(`[Api] Extending user access token ${url}`);
        const response = await fetch(url, {
            method: "PATCH",
            headers: maybeWithBearerAuth({}, session.token())
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async updateSettings(payload) {
        const url = accountSettingsUrl(config.baseUrl);
        const body = JSON.stringify(payload);
        console.log(`[Api] Updating user account ${url}: ${body}`);
        const response = await fetch(url, {
            method: "PATCH",
            headers: maybeWithBearerAuth({}, session.token()),
            body: body
        });
        if (response.status === 401 || response.status === 403) {
            throw new UnauthorizedError();
        } else if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
    }

    async addSubscription(payload) {
        const url = accountSubscriptionUrl(config.baseUrl);
        const body = JSON.stringify(payload);
        console.log(`[Api] Adding user subscription ${url}: ${body}`);
        const response = await fetch(url, {
            method: "POST",
            headers: maybeWithBearerAuth({}, session.token()),
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

    async deleteSubscription(remoteId) {
        const url = accountSubscriptionSingleUrl(config.baseUrl, remoteId);
        console.log(`[Api] Removing user subscription ${url}`);
        const response = await fetch(url, {
            method: "DELETE",
            headers: maybeWithBearerAuth({}, session.token())
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

const accountApi = new AccountApi();
export default accountApi;
