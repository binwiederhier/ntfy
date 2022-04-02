import {
    basicAuth,
    encodeBase64,
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

    async publish(baseUrl, topic, message, options) {
        const user = await userManager.get(baseUrl);
        console.log(`[Api] Publishing message to ${topicUrl(baseUrl, topic)}`);
        const headers = {};
        const body = {
            topic: topic,
            message: message,
            ...options
        };
        await fetch(baseUrl, {
            method: 'PUT',
            body: JSON.stringify(body),
            headers: maybeWithBasicAuth(headers, user)
        });
    }

    publishXHR(baseUrl, topic, body, headers, onProgress) {
        const url = topicUrl(baseUrl, topic);
        const xhr = new XMLHttpRequest();

        console.log(`[Api] Publishing message to ${url}`);
        const send = new Promise(function (resolve, reject) {
            xhr.open("PUT", url);
            xhr.addEventListener('readystatechange', (ev) => {
                console.log("read change", xhr.readyState, xhr.status, xhr.responseText, xhr)
                if (xhr.readyState === 4 && xhr.status >= 200 && xhr.status <= 299) {
                    console.log(`[Api] Publish successful (HTTP ${xhr.status})`, xhr.response);
                    resolve(xhr.response);
                } else if (xhr.readyState === 4) {
                    console.log(`[Api] Publish failed`, xhr.status, xhr.responseText, xhr);
                    xhr.abort();
                    reject(ev);
                }
            })
            xhr.upload.addEventListener("progress", onProgress);
            if (body.type) {
                xhr.overrideMimeType(body.type);
            }
            for (const [key, value] of Object.entries(headers)) {
                xhr.setRequestHeader(key, value);
            }
            xhr.send(body);
        });
        send.abort = () => {
            console.log(`[Api] Publish aborted by user`);
            xhr.abort();
        }
        return send;
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
