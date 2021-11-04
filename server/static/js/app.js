
/**
 * Hello, dear curious visitor. I am not a web-guy, so please don't judge my horrible JS code.
 * In fact, please do tell me about all the things I did wrong and that I could improve. I've been trying
 * to read up on modern JS, but it's just a little much.
 *
 * Feel free to open tickets at https://github.com/binwiederhier/ntfy/issues. Thank you!
 */

/* All the things */

let topics = {};

const topicsHeader = document.getElementById("topicsHeader");
const topicsList = document.getElementById("topicsList");
const topicField = document.getElementById("topicField");
const notifySound = document.getElementById("notifySound");
const subscribeButton = document.getElementById("subscribeButton");
const subscribeForm = document.getElementById("subscribeForm");
const errorField = document.getElementById("error");

const subscribe = (topic) => {
    if (Notification.permission !== "granted") {
        Notification.requestPermission().then((permission) => {
            if (permission === "granted") {
                subscribeInternal(topic, 0);
            } else {
                showNotificationDeniedError();
            }
        });
    } else {
        subscribeInternal(topic, 0);
    }
};

const subscribeInternal = (topic, delaySec) => {
    setTimeout(() => {
        // Render list entry
        let topicEntry = document.getElementById(`topic-${topic}`);
        if (!topicEntry) {
            topicEntry = document.createElement('li');
            topicEntry.id = `topic-${topic}`;
            topicEntry.innerHTML = `${topic} <button onclick="test('${topic}')"> <img src="static/img/send_black_24dp.svg"> Test</button> <button onclick="unsubscribe('${topic}')"> <img src="static/img/clear_black_24dp.svg"> Unsubscribe</button>`;
            topicsList.appendChild(topicEntry);
        }
        topicsHeader.style.display = '';

        // Open event source
        let eventSource = new EventSource(`${topic}/sse`);
        eventSource.onopen = () => {
            topicEntry.innerHTML = `${topic} <button onclick="test('${topic}')"> <img src="static/img/send_black_24dp.svg"> Test</button> <button onclick="unsubscribe('${topic}')"> <img src="static/img/clear_black_24dp.svg"> Unsubscribe</button>`;
            delaySec = 0; // Reset on successful connection
        };
        eventSource.onerror = (e) => {
            const newDelaySec = (delaySec + 5 <= 15) ? delaySec + 5 : 15;
            topicEntry.innerHTML = `${topic} <i>(Reconnecting in ${newDelaySec}s ...)</i> <button disabled="disabled">Test</button> <button onclick="unsubscribe('${topic}')">Unsubscribe</button>`;
            eventSource.close()
            subscribeInternal(topic, newDelaySec);
        };
        eventSource.onmessage = (e) => {
            const event = JSON.parse(e.data);
            notifySound.play();
            new Notification(`${location.host}/${topic}`, {
                body: event.message,
                icon: '/static/img/favicon.png'
            });
        };
        topics[topic] = eventSource;
        localStorage.setItem('topics', JSON.stringify(Object.keys(topics)));
    }, delaySec * 1000);
};

const unsubscribe = (topic) => {
    topics[topic].close();
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
        body: `This is a test notification sent from the Ntfy Web UI. It was sent at ${new Date().toString()}.`
    })
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

subscribeButton.onclick = function () {
    if (!topicField.value) {
        return false;
    }
    subscribe(topicField.value);
    topicField.value = "";
    return false;
};

// Disable Web UI if notifications of EventSource are not available
if (!window["Notification"] || !window["EventSource"]) {
    showBrowserIncompatibleError();
} else if (Notification.permission === "denied") {
    showNotificationDeniedError();
}

// Reset UI
topicField.value = "";

// Restore topics
const storedTopics = localStorage.getItem('topics');
if (storedTopics && Notification.permission === "granted") {
    const storedTopicsArray = JSON.parse(storedTopics)
    storedTopicsArray.forEach((topic) => { subscribeInternal(topic, 0); });
    if (storedTopicsArray.length === 0) {
        topicsHeader.style.display = 'none';
    }
} else {
    topicsHeader.style.display = 'none';
}
