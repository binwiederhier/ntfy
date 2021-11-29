
/**
 * Hello, dear curious visitor. I am not a web-guy, so please don't judge my horrible JS code.
 * In fact, please do tell me about all the things I did wrong and that I could improve. I've been trying
 * to read up on modern JS, but it's just a little much.
 *
 * Feel free to open tickets at https://github.com/binwiederhier/ntfy/issues. Thank you!
 */

/* All the things */

let topics = {};
let currentTopic = "";
let currentTopicUnsubscribeOnClose = false;
let currentUrl = window.location.hostname;
if (window.location.port) {
    currentUrl += ':' + window.location.port
}

/* Main view */
const main = document.getElementById("main");
const topicsHeader = document.getElementById("topicsHeader");
const topicsList = document.getElementById("topicsList");
const topicField = document.getElementById("topicField");
const notifySound = document.getElementById("notifySound");
const subscribeButton = document.getElementById("subscribeButton");
const errorField = document.getElementById("error");
const originalTitle = document.title;

/* Detail view */
const detailView = document.getElementById("detail");
const detailTitle = document.getElementById("detailTitle");
const detailEventsList = document.getElementById("detailEventsList");
const detailTopicUrl = document.getElementById("detailTopicUrl");
const detailNoNotifications = document.getElementById("detailNoNotifications");
const detailCloseButton = document.getElementById("detailCloseButton");
const detailNotificationsDisallowed = document.getElementById("detailNotificationsDisallowed");

/* Screenshots */
const lightbox = document.getElementById("lightbox");

const subscribe = (topic) => {
    if (Notification.permission !== "granted") {
        Notification.requestPermission().then((permission) => {
            if (permission === "granted") {
                subscribeInternal(topic, true, 0);
            } else {
                showNotificationDeniedError();
            }
        });
    } else {
        subscribeInternal(topic, true,0);
    }
};

const subscribeInternal = (topic, persist, delaySec) => {
    setTimeout(() => {
        // Render list entry
        let topicEntry = document.getElementById(`topic-${topic}`);
        if (!topicEntry) {
            topicEntry = document.createElement('li');
            topicEntry.id = `topic-${topic}`;
            topicEntry.innerHTML = `<a href="/${topic}" onclick="return showDetail('${topic}')">${topic}</a> <button onclick="test('${topic}'); return false;"> <img src="static/img/send.svg"> Test</button> <button onclick="unsubscribe('${topic}'); return false;"> <img src="static/img/unsubscribe.svg"> Unsubscribe</button>`;
            topicsList.appendChild(topicEntry);
        }
        topicsHeader.style.display = '';

        // Open event source
        let eventSource = new EventSource(`${topic}/sse`);
        eventSource.onopen = () => {
            topicEntry.innerHTML = `<a href="/${topic}" onclick="return showDetail('${topic}')">${topic}</a> <button onclick="test('${topic}'); return false;"> <img src="static/img/send.svg"> Test</button> <button onclick="unsubscribe('${topic}'); return false;"> <img src="static/img/unsubscribe.svg"> Unsubscribe</button>`;
            delaySec = 0; // Reset on successful connection
        };
        eventSource.onerror = (e) => {
            topicEntry.innerHTML = `<a href="/${topic}" onclick="return showDetail('${topic}')">${topic}</a> <i>(Reconnecting)</i> <button disabled="disabled">Test</button> <button onclick="unsubscribe('${topic}'); return false;">Unsubscribe</button>`;
            eventSource.close();
            const newDelaySec = (delaySec + 5 <= 15) ? delaySec + 5 : 15;
            subscribeInternal(topic, persist, newDelaySec);
        };
        eventSource.onmessage = (e) => {
            const event = JSON.parse(e.data);
            topics[topic]['messages'].push(event);
            topics[topic]['messages'].sort((a, b) => { return a.time < b.time ? 1 : -1; }); // Newest first
            if (currentTopic === topic) {
                rerenderDetailView();
            }
            if (Notification.permission === "granted") {
                notifySound.play();
                const title = (event.title) ? event.title : `${location.host}/${topic}`;
                const notification = new Notification(title, {
                    body: event.message,
                    icon: '/static/img/favicon.png'
                });
                notification.onclick = (e) => {
                    showDetail(event.topic);
                };
            }
        };
        topics[topic] = {
            'eventSource': eventSource,
            'messages': [],
            'persist': persist
        };
        fetchCachedMessages(topic).then(() => {
            if (currentTopic === topic) {
                rerenderDetailView();
            }
        })
        let persistedTopicKeys = Object.keys(topics).filter(t => topics[t].persist);
        localStorage.setItem('topics', JSON.stringify(persistedTopicKeys));
    }, delaySec * 1000);
};

const unsubscribe = (topic) => {
    topics[topic]['eventSource'].close();
    delete topics[topic];
    localStorage.setItem('topics', JSON.stringify(Object.keys(topics)));
    document.getElementById(`topic-${topic}`).remove();
    if (Object.keys(topics).length === 0) {
        topicsHeader.style.display = 'none';
    }
};

const test = (topic) => {
    fetch(`/${topic}`, {
        method: 'PUT',
        body: `This is a test notification sent by the ntfy.sh Web UI at ${new Date().toString()}.`
    });
};

const fetchCachedMessages = async (topic) => {
    const topicJsonUrl = `/${topic}/json?poll=1`; // Poll!
    for await (let line of makeTextFileLineIterator(topicJsonUrl)) {
        const message = JSON.parse(line);
        topics[topic]['messages'].push(message);
    }
    topics[topic]['messages'].sort((a, b) => { return a.time < b.time ? 1 : -1; }); // Newest first
};

const showDetail = (topic) => {
    currentTopic = topic;
    history.replaceState(topic, `${currentUrl}/${topic}`, `/${topic}`);
    window.scrollTo(0, 0);
    rerenderDetailView();
    return false;
};

const rerenderDetailView = () => {
    detailTitle.innerHTML = `${currentUrl}/${currentTopic}`; // document.location.replaceAll(..)
    detailTopicUrl.innerHTML = `${currentUrl}/${currentTopic}`;
    while (detailEventsList.firstChild) {
        detailEventsList.removeChild(detailEventsList.firstChild);
    }
    topics[currentTopic]['messages'].forEach(m => {
        const entryDiv = document.createElement('div');
        const dateDiv = document.createElement('div');
        const titleDiv = document.createElement('div');
        const messageDiv = document.createElement('div');
        const tagsDiv = document.createElement('div');

        // Figure out mapped emojis (and unmapped tags)
        let mappedEmojiTags = '';
        let unmappedTags = '';
        if (m.tags) {
            mappedEmojiTags = m.tags
                .filter(tag => tag in emojis)
                .map(tag => emojis[tag])
                .join("");
            unmappedTags = m.tags
                .filter(tag => !(tag in emojis))
                .join(", ");
        }

        // Figure out title and message
        let title = '';
        let message = m.message;
        if (m.title) {
            if (mappedEmojiTags) {
                title = `${mappedEmojiTags} ${m.title}`;
            } else {
                title = m.title;
            }
        } else {
            if (mappedEmojiTags) {
                message = `${mappedEmojiTags} ${m.message}`;
            } else {
                message = m.message;
            }
        }

        entryDiv.classList.add('detailEntry');
        dateDiv.classList.add('detailDate');
        const dateStr = new Date(m.time * 1000).toLocaleString();
        if (m.priority && [1,2,4,5].includes(m.priority)) {
            dateDiv.innerHTML = `${dateStr} <img src="static/img/priority-${m.priority}.svg"/>`;
        } else {
            dateDiv.innerHTML = `${dateStr}`;
        }
        messageDiv.classList.add('detailMessage');
        messageDiv.innerText = message;
        entryDiv.appendChild(dateDiv);
        if (m.title) {
            titleDiv.classList.add('detailTitle');
            titleDiv.innerText = title;
            entryDiv.appendChild(titleDiv);
        }
        entryDiv.appendChild(messageDiv);
        if (unmappedTags) {
            tagsDiv.classList.add('detailTags');
            tagsDiv.innerText = `Tags: ${unmappedTags}`;
            entryDiv.appendChild(tagsDiv);
        }
        detailEventsList.appendChild(entryDiv);
    })
    if (topics[currentTopic]['messages'].length === 0) {
        detailNoNotifications.style.display = '';
    } else {
        detailNoNotifications.style.display = 'none';
    }
    if (Notification.permission === "granted") {
        detailNotificationsDisallowed.style.display = 'none';
    } else {
        detailNotificationsDisallowed.style.display = 'block';
    }
    detailView.style.display = 'block';
    main.style.display = 'none';
};

const hideDetailView = () => {
    if (currentTopicUnsubscribeOnClose) {
        unsubscribe(currentTopic);
        currentTopicUnsubscribeOnClose = false;
    }
    currentTopic = "";
    history.replaceState('', originalTitle, '/');
    detailView.style.display = 'none';
    main.style.display = 'block';
    return false;
};

const requestPermission = () => {
    if (Notification.permission !== "granted") {
        Notification.requestPermission().then((permission) => {
            if (permission === "granted") {
                detailNotificationsDisallowed.style.display = 'none';
            }
        });
    }
    return false;
};

const showError = (msg) => {
    errorField.innerHTML = msg;
    topicField.disabled = true;
    subscribeButton.disabled = true;
};

const showBrowserIncompatibleError = () => {
    showError("Your browser is not compatible to use the web-based desktop notifications.");
};

const showNotificationDeniedError = () => {
    showError("You have blocked desktop notifications for this website. Please unblock them and refresh to use the web-based desktop notifications.");
};

const showScreenshotOverlay = (e, el, index) => {
    lightbox.classList.add('show');
    document.addEventListener('keydown', nextScreenshotKeyboardListener);
    return showScreenshot(e, index);
};

const showScreenshot = (e, index) => {
    const actualIndex = resolveScreenshotIndex(index);
    lightbox.innerHTML = '<div class="close-lightbox"></div>' + screenshots[actualIndex].innerHTML;
    lightbox.querySelector('img').onclick = (e) => { return showScreenshot(e,actualIndex+1); };
    currentScreenshotIndex = actualIndex;
    e.stopPropagation();
    return false;
};

const nextScreenshot = (e) => {
    return showScreenshot(e, currentScreenshotIndex+1);
};

const previousScreenshot = (e) => {
    return showScreenshot(e, currentScreenshotIndex-1);
};

const resolveScreenshotIndex = (index) => {
    if (index < 0) {
        return screenshots.length - 1;
    } else if (index > screenshots.length - 1) {
        return 0;
    }
    return index;
};

const hideScreenshotOverlay = (e) => {
    lightbox.classList.remove('show');
    document.removeEventListener('keydown', nextScreenshotKeyboardListener);
};

const nextScreenshotKeyboardListener = (e) => {
    switch (e.keyCode) {
        case 37:
            previousScreenshot(e);
            break;
        case 39:
            nextScreenshot(e);
            break;
    }
};

const toEmoji = (tag) => {
    emojis
};

// From: https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch
async function* makeTextFileLineIterator(fileURL) {
    const utf8Decoder = new TextDecoder('utf-8');
    const response = await fetch(fileURL);
    const reader = response.body.getReader();
    let { value: chunk, done: readerDone } = await reader.read();
    chunk = chunk ? utf8Decoder.decode(chunk) : '';

    const re = /\n|\r|\r\n/gm;
    let startIndex = 0;
    let result;

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

subscribeButton.onclick = () => {
    if (!topicField.value) {
        return false;
    }
    subscribe(topicField.value);
    topicField.value = "";
    return false;
};

detailCloseButton.onclick = () => {
    hideDetailView();
};

let currentScreenshotIndex = 0;
const screenshots = [...document.querySelectorAll("#screenshots a")];
screenshots.forEach((el, index) => {
    el.onclick = (e) => { return showScreenshotOverlay(e, el, index); };
});

lightbox.onclick = hideScreenshotOverlay;

// Disable Web UI if notifications of EventSource are not available
if (!window["Notification"] || !window["EventSource"]) {
    showBrowserIncompatibleError();
} else if (Notification.permission === "denied") {
    showNotificationDeniedError();
}

// Reset UI
topicField.value = "";

// Restore topics
const storedTopics = JSON.parse(localStorage.getItem('topics') || "[]");
if (storedTopics) {
    storedTopics.forEach((topic) => { subscribeInternal(topic, true, 0); });
    if (storedTopics.length === 0) {
        topicsHeader.style.display = 'none';
    }
} else {
    topicsHeader.style.display = 'none';
}

// (Temporarily) subscribe topic if we navigated to /sometopic URL
const match = location.pathname.match(/^\/([-_a-zA-Z0-9]{1,64})$/) // Regex must match Go & Android app!
if (match) {
    currentTopic = match[1];
    if (!storedTopics.includes(currentTopic)) {
        subscribeInternal(currentTopic, false,0);
        currentTopicUnsubscribeOnClose = true;
    }
}

// Add anchor links
document.querySelectorAll('.anchor').forEach((el) => {
    if (el.hasAttribute('id')) {
        const id = el.getAttribute('id');
        const anchor = document.createElement('a');
        anchor.innerHTML = `<a href="#${id}" class="anchorLink">#</a>`;
        el.appendChild(anchor);
    }
});

// Change ntfy.sh url and protocol to match self-hosted one
document.querySelectorAll('.ntfyUrl').forEach((el) => {
    el.innerHTML = currentUrl;
});
document.querySelectorAll('.ntfyProtocol').forEach((el) => {
    el.innerHTML = window.location.protocol + "//";
});

// Fetch emojis
const emojis = {};
fetch('static/js/emoji.json')
    .then(response => response.json())
    .then(data => {
        data.forEach(emoji => {
            emoji.aliases.forEach(alias => {
                emojis[alias] = emoji.emoji;
            });
        });
    });
