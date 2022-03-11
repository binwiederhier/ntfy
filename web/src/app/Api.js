import {
    fetchLinesIterator,
    maybeWithBasicAuth,
    topicShortUrl,
    topicUrl,
    topicUrlAuth,
    topicUrlJsonPoll,
    topicUrlJsonPollWithSince
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

    async publish(baseUrl, topic, message, title, priority, tags) {
        const user = await userManager.get(baseUrl);
        const url = topicUrl(baseUrl, topic);
        console.log(`[Api] Publishing message to ${url}`);
        const headers = {};
        if (title) {
            headers["X-Title"] = title;
        }
        if (priority !== 3) {
            headers["X-Priority"] = `${priority}`;
        }
        if (tags.length > 0) {
            headers["X-Tags"] = tags.join(",");
        }
        await fetch(url, {
            method: 'PUT',
            body: message,
            headers: maybeWithBasicAuth(headers, user)
        });
    }

    async auth(baseUrl, topic, user) {
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
}

const api = new Api();
export default api;
