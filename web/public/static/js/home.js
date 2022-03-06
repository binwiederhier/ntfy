
/**
 * Hello, dear curious visitor. I am not a web-guy, so please don't judge my horrible JS code.
 * In fact, please do tell me about all the things I did wrong and that I could improve. I've been trying
 * to read up on modern JS, but it's just a little much.
 *
 * Feel free to open tickets at https://github.com/binwiederhier/ntfy/issues. Thank you!
 */

/* All the things */

let currentUrl = window.location.hostname;
if (window.location.port) {
    currentUrl += ':' + window.location.port
}

/* Screenshots */
const lightbox = document.getElementById("lightbox");

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

let currentScreenshotIndex = 0;
const screenshots = [...document.querySelectorAll("#screenshots a")];
screenshots.forEach((el, index) => {
    el.onclick = (e) => { return showScreenshotOverlay(e, el, index); };
});

lightbox.onclick = hideScreenshotOverlay;

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
