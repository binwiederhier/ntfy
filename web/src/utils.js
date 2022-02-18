export const topicUrl = (baseUrl, topic) => `${baseUrl}/${topic}`;
export const topicUrlWs = (baseUrl, topic) => `${topicUrl(baseUrl, topic)}/ws`
    .replaceAll("https://", "wss://")
    .replaceAll("http://", "ws://");
export const shortUrl = (url) => url.replaceAll(/https?:\/\//g, "");
export const shortTopicUrl = (baseUrl, topic) => shortUrl(topicUrl(baseUrl, topic));
