import {
    fetchLinesIterator,
    maybeWithBasicAuth,
    topicShortUrl,
    topicUrl,
    topicUrlAuth,
    topicUrlJsonPoll,
    topicUrlJsonPollWithSince,
    userStatsUrl
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

    async userStats(baseUrl) {
        const url = userStatsUrl(baseUrl);
        console.log(`[Api] Fetching user stats ${url}`);
        const response = await fetch(url);
        if (response.status !== 200) {
            throw new Error(`Unexpected server response ${response.status}`);
        }
        const stats = await response.json();
        console.log(`[Api] Stats`, stats);
        return stats;
    }
}

const api = new Api();
export default api;
