import {
    accountBillingPortalUrl,
    accountBillingSubscriptionUrl,
    accountPasswordUrl, accountPhoneUrl, accountPhoneVerifyUrl,
    accountReservationSingleUrl,
    accountReservationUrl,
    accountSettingsUrl,
    accountSubscriptionSingleUrl,
    accountSubscriptionUrl,
    accountTokenUrl,
    accountUrl, maybeWithBearerAuth,
    tiersUrl,
    withBasicAuth,
    withBearerAuth
} from "./utils";
import session from "./Session";
import subscriptionManager from "./SubscriptionManager";
import i18n from "i18next";
import prefs from "./Prefs";
import routes from "../components/routes";
import {fetchOrThrow, throwAppError, UnauthorizedError} from "./errors";

const delayMillis = 45000; // 45 seconds
const intervalMillis = 900000; // 15 minutes

class AccountApi {
    constructor() {
        this.timer = null;
        this.listener = null; // Fired when account is fetched from remote
        this.tiers = null; // Cached
    }

    registerListener(listener) {
        this.listener = listener;
    }

    resetListener() {
        this.listener = null;
    }

    async login(user) {
        const url = accountTokenUrl(config.base_url);
        console.log(`[AccountApi] Checking auth for ${url}`);
        const response = await fetchOrThrow(url, {
            method: "POST",
            headers: withBasicAuth({}, user.username, user.password)
        });
        const json = await response.json(); // May throw SyntaxError
        if (!json.token) {
            throw new Error(`Unexpected server response: Cannot find token`);
        }
        return json.token;
    }

    async logout() {
        const url = accountTokenUrl(config.base_url);
        console.log(`[AccountApi] Logging out from ${url} using token ${session.token()}`);
        await fetchOrThrow(url, {
            method: "DELETE",
            headers: withBearerAuth({}, session.token())
        });
    }

    async create(username, password) {
        const url = accountUrl(config.base_url);
        const body = JSON.stringify({
            username: username,
            password: password
        });
        console.log(`[AccountApi] Creating user account ${url}`);
        await fetchOrThrow(url, {
            method: "POST",
            body: body
        });
    }

    async get() {
        const url = accountUrl(config.base_url);
        console.log(`[AccountApi] Fetching user account ${url}`);
        const response = await fetchOrThrow(url, {
            headers: maybeWithBearerAuth({}, session.token()) // GET /v1/account endpoint can be called by anonymous
        });
        const account = await response.json(); // May throw SyntaxError
        console.log(`[AccountApi] Account`, account);
        if (this.listener) {
            this.listener(account);
        }
        return account;
    }

    async delete(password) {
        const url = accountUrl(config.base_url);
        console.log(`[AccountApi] Deleting user account ${url}`);
        await fetchOrThrow(url, {
            method: "DELETE",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify({
                password: password
            })
        });
    }

    async changePassword(currentPassword, newPassword) {
        const url = accountPasswordUrl(config.base_url);
        console.log(`[AccountApi] Changing account password ${url}`);
        await fetchOrThrow(url, {
            method: "POST",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify({
                password: currentPassword,
                new_password: newPassword
            })
        });
    }

    async createToken(label, expires) {
        const url = accountTokenUrl(config.base_url);
        const body = {
            label: label,
            expires: (expires > 0) ? Math.floor(Date.now() / 1000) + expires : 0
        };
        console.log(`[AccountApi] Creating user access token ${url}`);
        await fetchOrThrow(url, {
            method: "POST",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify(body)
        });
    }

    async updateToken(token, label, expires) {
        const url = accountTokenUrl(config.base_url);
        const body = {
            token: token,
            label: label
        };
        if (expires > 0) {
            body.expires = Math.floor(Date.now() / 1000) + expires;
        }
        console.log(`[AccountApi] Creating user access token ${url}`);
        await fetchOrThrow(url, {
            method: "PATCH",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify(body)
        });
    }

    async extendToken() {
        const url = accountTokenUrl(config.base_url);
        console.log(`[AccountApi] Extending user access token ${url}`);
        await fetchOrThrow(url, {
            method: "PATCH",
            headers: withBearerAuth({}, session.token())
        });
    }

    async deleteToken(token) {
        const url = accountTokenUrl(config.base_url);
        console.log(`[AccountApi] Deleting user access token ${url}`);
        await fetchOrThrow(url, {
            method: "DELETE",
            headers: withBearerAuth({"X-Token": token}, session.token())
        });
    }

    async updateSettings(payload) {
        const url = accountSettingsUrl(config.base_url);
        const body = JSON.stringify(payload);
        console.log(`[AccountApi] Updating user account ${url}: ${body}`);
        await fetchOrThrow(url, {
            method: "PATCH",
            headers: withBearerAuth({}, session.token()),
            body: body
        });
    }

    async addSubscription(baseUrl, topic) {
        const url = accountSubscriptionUrl(config.base_url);
        const body = JSON.stringify({
            base_url: baseUrl,
            topic: topic
        });
        console.log(`[AccountApi] Adding user subscription ${url}: ${body}`);
        const response = await fetchOrThrow(url, {
            method: "POST",
            headers: withBearerAuth({}, session.token()),
            body: body
        });
        const subscription = await response.json(); // May throw SyntaxError
        console.log(`[AccountApi] Subscription`, subscription);
        return subscription;
    }

    async updateSubscription(baseUrl, topic, payload) {
        const url = accountSubscriptionUrl(config.base_url);
        const body = JSON.stringify({
            base_url: baseUrl,
            topic: topic,
            ...payload
        });
        console.log(`[AccountApi] Updating user subscription ${url}: ${body}`);
        const response = await fetchOrThrow(url, {
            method: "PATCH",
            headers: withBearerAuth({}, session.token()),
            body: body
        });
        const subscription = await response.json(); // May throw SyntaxError
        console.log(`[AccountApi] Subscription`, subscription);
        return subscription;
    }

    async deleteSubscription(baseUrl, topic) {
        const url = accountSubscriptionUrl(config.base_url);
        console.log(`[AccountApi] Removing user subscription ${url}`);
        const headers = {
            "X-BaseURL": baseUrl,
            "X-Topic":  topic,
        }
        await fetchOrThrow(url, {
            method: "DELETE",
            headers: withBearerAuth(headers, session.token()),
        });
    }

    async upsertReservation(topic, everyone) {
        const url = accountReservationUrl(config.base_url);
        console.log(`[AccountApi] Upserting user access to topic ${topic}, everyone=${everyone}`);
        await fetchOrThrow(url, {
            method: "POST",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify({
                topic: topic,
                everyone: everyone
            })
        });
    }

    async deleteReservation(topic, deleteMessages) {
        const url = accountReservationSingleUrl(config.base_url, topic);
        console.log(`[AccountApi] Removing topic reservation ${url}`);
        const headers = {
            "X-Delete-Messages": deleteMessages ? "true" : "false"
        }
        await fetchOrThrow(url, {
            method: "DELETE",
            headers: withBearerAuth(headers, session.token())
        });
    }

    async billingTiers() {
        if (this.tiers) {
            return this.tiers;
        }
        const url = tiersUrl(config.base_url);
        console.log(`[AccountApi] Fetching billing tiers`);
        const response = await fetchOrThrow(url); // No auth needed!
        this.tiers = await response.json(); // May throw SyntaxError
        return this.tiers;
    }

    async createBillingSubscription(tier, interval) {
        console.log(`[AccountApi] Creating billing subscription with ${tier} and interval ${interval}`);
        return await this.upsertBillingSubscription("POST", tier, interval)
    }

    async updateBillingSubscription(tier, interval) {
        console.log(`[AccountApi] Updating billing subscription with ${tier} and interval ${interval}`);
        return await this.upsertBillingSubscription("PUT", tier, interval)
    }

    async upsertBillingSubscription(method, tier, interval) {
        const url = accountBillingSubscriptionUrl(config.base_url);
        const response = await fetchOrThrow(url, {
            method: method,
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify({
                tier: tier,
                interval: interval
            })
        });
        return await response.json(); // May throw SyntaxError
    }

    async deleteBillingSubscription() {
        const url = accountBillingSubscriptionUrl(config.base_url);
        console.log(`[AccountApi] Cancelling billing subscription`);
        await fetchOrThrow(url, {
            method: "DELETE",
            headers: withBearerAuth({}, session.token())
        });
    }

    async createBillingPortalSession() {
        const url = accountBillingPortalUrl(config.base_url);
        console.log(`[AccountApi] Creating billing portal session`);
        const response = await fetchOrThrow(url, {
            method: "POST",
            headers: withBearerAuth({}, session.token())
        });
        return await response.json(); // May throw SyntaxError
    }

    async verifyPhoneNumber(phoneNumber) {
        const url = accountPhoneVerifyUrl(config.base_url);
        console.log(`[AccountApi] Sending phone verification ${url}`);
        await fetchOrThrow(url, {
            method: "PUT",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify({
                number: phoneNumber
            })
        });
    }

    async addPhoneNumber(phoneNumber, code) {
        const url = accountPhoneUrl(config.base_url);
        console.log(`[AccountApi] Adding phone number with verification code ${url}`);
        await fetchOrThrow(url, {
            method: "PUT",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify({
                number: phoneNumber,
                code: code
            })
        });
    }

    async deletePhoneNumber(phoneNumber, code) {
        const url = accountPhoneUrl(config.base_url);
        console.log(`[AccountApi] Deleting phone number ${url}`);
        await fetchOrThrow(url, {
            method: "DELETE",
            headers: withBearerAuth({}, session.token()),
            body: JSON.stringify({
                number: phoneNumber
            })
        });
    }

    async sync() {
        try {
            if (!session.token()) {
                return null;
            }
            console.log(`[AccountApi] Syncing account`);
            const account = await this.get();
            if (account.language) {
                await i18n.changeLanguage(account.language);
            }
            if (account.notification) {
                if (account.notification.sound) {
                    await prefs.setSound(account.notification.sound);
                }
                if (account.notification.delete_after) {
                    await prefs.setDeleteAfter(account.notification.delete_after);
                }
                if (account.notification.min_priority) {
                    await prefs.setMinPriority(account.notification.min_priority);
                }
            }
            if (account.subscriptions) {
                await subscriptionManager.syncFromRemote(account.subscriptions, account.reservations);
            }
            return account;
        } catch (e) {
            console.log(`[AccountApi] Error fetching account`, e);
            if (e instanceof UnauthorizedError) {
                session.resetAndRedirect(routes.login);
            }
        }
    }

    startWorker() {
        if (this.timer !== null) {
            return;
        }
        console.log(`[AccountApi] Starting worker`);
        this.timer = setInterval(() => this.runWorker(), intervalMillis);
        setTimeout(() => this.runWorker(), delayMillis);
    }

    async runWorker() {
        if (!session.token()) {
            return;
        }
        console.log(`[AccountApi] Extending user access token`);
        try {
            await this.extendToken();
        } catch (e) {
            console.log(`[AccountApi] Error extending user access token`, e);
        }
    }
}

// Maps to user.Role in user/types.go
export const Role = {
    ADMIN: "admin",
    USER: "user"
};

// Maps to server.visitorLimitBasis in server/visitor.go
export const LimitBasis = {
    IP: "ip",
    TIER: "tier"
};

// Maps to stripe.SubscriptionStatus
export const SubscriptionStatus = {
    ACTIVE: "active",
    PAST_DUE: "past_due"
};

// Maps to stripe.PriceRecurringInterval
export const SubscriptionInterval = {
    MONTH: "month",
    YEAR: "year"
};

// Maps to user.Permission in user/types.go
export const Permission = {
    READ_WRITE: "read-write",
    READ_ONLY: "read-only",
    WRITE_ONLY: "write-only",
    DENY_ALL: "deny-all"
};

const accountApi = new AccountApi();
export default accountApi;
