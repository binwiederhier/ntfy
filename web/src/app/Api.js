import {topicUrlJsonPoll, fetchLinesIterator, topicUrl, topicUrlAuth} from "./utils";

class Api {
    async poll(baseUrl, topic) {
        const url = topicUrlJsonPoll(baseUrl, topic);
        const messages = [];
        console.log(`[Api] Polling ${url}`);
        for await (let line of fetchLinesIterator(url)) {
            messages.push(JSON.parse(line));
        }
        return messages;
    }

    async publish(baseUrl, topic, message) {
        const url = topicUrl(baseUrl, topic);
        console.log(`[Api] Publishing message to ${url}`);
        await fetch(url, {
            method: 'PUT',
            body: message
        });
    }

    async auth(baseUrl, topic, user) {
        const url = topicUrlAuth(baseUrl, topic);
        console.log(`[Api] Checking auth for ${url}`);
        const response = await fetch(url);
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
