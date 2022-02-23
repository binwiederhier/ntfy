import {topicUrlJsonPoll, fetchLinesIterator, topicUrl} from "./utils";

class Api {
    static async poll(baseUrl, topic) {
        const url = topicUrlJsonPoll(baseUrl, topic);
        const messages = [];
        console.log(`[Api] Polling ${url}`);
        for await (let line of fetchLinesIterator(url)) {
            messages.push(JSON.parse(line));
        }
        return messages.sort((a, b) => { return a.time < b.time ? 1 : -1; }); // Newest first
    }

    static async publish(baseUrl, topic, message) {
        const url = topicUrl(baseUrl, topic);
        console.log(`[Api] Publishing message to ${url}`);
        await fetch(url, {
            method: 'PUT',
            body: message
        });
    }
}

export default Api;
