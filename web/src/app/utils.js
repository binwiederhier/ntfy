import {rawEmojis} from "./emojis";
import beep from "../sounds/beep.mp3";
import juntos from "../sounds/juntos.mp3";
import pristine from "../sounds/pristine.mp3";
import ding from "../sounds/ding.mp3";
import dadum from "../sounds/dadum.mp3";
import pop from "../sounds/pop.mp3";
import popSwoosh from "../sounds/pop-swoosh.mp3";
import config from "./config";
import {Base64} from 'js-base64';

export const topicUrl = (baseUrl, topic) => `${baseUrl}/${topic}`;
export const topicUrlWs = (baseUrl, topic) => `${topicUrl(baseUrl, topic)}/ws`
    .replaceAll("https://", "wss://")
    .replaceAll("http://", "ws://");
export const topicUrlJson = (baseUrl, topic) => `${topicUrl(baseUrl, topic)}/json`;
export const topicUrlJsonPoll = (baseUrl, topic) => `${topicUrlJson(baseUrl, topic)}?poll=1`;
export const topicUrlJsonPollWithSince = (baseUrl, topic, since) => `${topicUrlJson(baseUrl, topic)}?poll=1&since=${since}`;
export const topicUrlAuth = (baseUrl, topic) => `${topicUrl(baseUrl, topic)}/auth`;
export const topicShortUrl = (baseUrl, topic) => shortUrl(topicUrl(baseUrl, topic));
export const userStatsUrl = (baseUrl) => `${baseUrl}/user/stats`;
export const userTokenUrl = (baseUrl) => `${baseUrl}/user/token`;
export const userAccountUrl = (baseUrl) => `${baseUrl}/user/account`;
export const userSubscriptionUrl = (baseUrl) => `${baseUrl}/user/subscription`;
export const userSubscriptionDeleteUrl = (baseUrl, id) => `${baseUrl}/user/subscription/${id}`;
export const shortUrl = (url) => url.replaceAll(/https?:\/\//g, "");
export const expandUrl = (url) => [`https://${url}`, `http://${url}`];
export const expandSecureUrl = (url) => `https://${url}`;

export const validUrl = (url) => {
    return url.match(/^https?:\/\/.+/);
}

export const validTopic = (topic) => {
    if (disallowedTopic(topic)) {
        return false;
    }
    return topic.match(/^([-_a-zA-Z0-9]{1,64})$/); // Regex must match Go & Android app!
}

export const disallowedTopic = (topic) => {
    return config.disallowedTopics.includes(topic);
}

export const topicDisplayName = (subscription) => {
    if (subscription.displayName) {
        return subscription.displayName;
    } else if (subscription.baseUrl === window.location.origin) {
        return subscription.topic;
    }
    return topicShortUrl(subscription.baseUrl, subscription.topic);
};

// Format emojis (see emoji.js)
const emojis = {};
rawEmojis.forEach(emoji => {
    emoji.aliases.forEach(alias => {
        emojis[alias] = emoji.emoji;
    });
});

const toEmojis = (tags) => {
    if (!tags) return [];
    else return tags.filter(tag => tag in emojis).map(tag => emojis[tag]);
}

export const formatTitleWithDefault = (m, fallback) => {
    if (m.title) {
        return formatTitle(m);
    }
    return fallback;
};

export const formatTitle = (m) => {
    const emojiList = toEmojis(m.tags);
    if (emojiList.length > 0) {
        return `${emojiList.join(" ")} ${m.title}`;
    } else {
        return m.title;
    }
};

export const formatMessage = (m) => {
    if (m.title) {
        return m.message;
    } else {
        const emojiList = toEmojis(m.tags);
        if (emojiList.length > 0) {
            return `${emojiList.join(" ")} ${m.message}`;
        } else {
            return m.message;
        }
    }
};

export const unmatchedTags = (tags) => {
    if (!tags) return [];
    else return tags.filter(tag => !(tag in emojis));
}

export const maybeWithBasicAuth = (headers, user) => {
    if (user) {
        headers['Authorization'] = `Basic ${encodeBase64(`${user.username}:${user.password}`)}`;
    }
    return headers;
}

export const maybeWithBearerAuth = (headers, token) => {
    if (token) {
        headers['Authorization'] = `Bearer ${token}`;
    }
    return headers;
}

export const basicAuth = (username, password) => {
    return `Basic ${encodeBase64(`${username}:${password}`)}`;
}

export const encodeBase64 = (s) => {
    return Base64.encode(s);
}

export const encodeBase64Url = (s) => {
    return Base64.encodeURI(s);
}

export const maybeAppendActionErrors = (message, notification) => {
    const actionErrors = (notification.actions ?? [])
        .map(action => action.error)
        .filter(action => !!action)
        .join("\n")
    if (actionErrors.length === 0) {
        return message;
    } else {
        return `${message}\n\n${actionErrors}`;
    }
}

export const shuffle = (arr) => {
    let j, x;
    for (let index = arr.length - 1; index > 0; index--) {
        j = Math.floor(Math.random() * (index + 1));
        x = arr[index];
        arr[index] = arr[j];
        arr[j] = x;
    }
    return arr;
}

export const splitNoEmpty = (s, delimiter) => {
    return s
        .split(delimiter)
        .map(x => x.trim())
        .filter(x => x !== "");
}

/** Non-cryptographic hash function, see https://stackoverflow.com/a/8831937/1440785 */
export const hashCode = async (s) => {
    let hash = 0;
    for (let i = 0; i < s.length; i++) {
        const char = s.charCodeAt(i);
        hash = ((hash<<5)-hash)+char;
        hash = hash & hash; // Convert to 32bit integer
    }
    return hash;
}

export const formatShortDateTime = (timestamp) => {
    return new Intl.DateTimeFormat('default', {dateStyle: 'short', timeStyle: 'short'})
        .format(new Date(timestamp * 1000));
}

export const formatBytes = (bytes, decimals = 2) => {
    if (bytes === 0) return '0 bytes';
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

export const openUrl = (url) => {
    window.open(url, "_blank", "noopener,noreferrer");
};

export const sounds = {
    "ding": {
        file: ding,
        label: "Ding"
    },
    "juntos": {
        file: juntos,
        label: "Juntos"
    },
    "pristine": {
        file: pristine,
        label: "Pristine"
    },
    "dadum": {
        file: dadum,
        label: "Dadum"
    },
    "pop": {
        file: pop,
        label: "Pop"
    },
    "pop-swoosh": {
        file: popSwoosh,
        label: "Pop swoosh"
    },
    "beep": {
        file: beep,
        label: "Beep"
    }
};

export const playSound = async (id) => {
    const audio = new Audio(sounds[id].file);
    return audio.play();
};

// From: https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch
export async function* fetchLinesIterator(fileURL, headers) {
    const utf8Decoder = new TextDecoder('utf-8');
    const response = await fetch(fileURL, {
        headers: headers
    });
    const reader = response.body.getReader();
    let { value: chunk, done: readerDone } = await reader.read();
    chunk = chunk ? utf8Decoder.decode(chunk) : '';

    const re = /\n|\r|\r\n/gm;
    let startIndex = 0;

    for (;;) {
        let result = re.exec(chunk);
        if (!result) {
            if (readerDone) {
                break;
            }
            let remainder = chunk.substr(startIndex);
            ({ value: chunk, done: readerDone } = await reader.read());
            chunk = remainder + (chunk ? utf8Decoder.decode(chunk) : '');
            startIndex = re.lastIndex = 0;
            continue;
        }
        yield chunk.substring(startIndex, result.index);
        startIndex = re.lastIndex;
    }
    if (startIndex < chunk.length) {
        yield chunk.substr(startIndex); // last line didn't end in a newline char
    }
}

export const randomAlphanumericString = (len) => {
    const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789";
    let id = "";
    for (let i = 0; i < len; i++) {
        id += alphabet[(Math.random() * alphabet.length) | 0];
    }
    return id;
}
