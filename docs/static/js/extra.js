// Link tabs, as per https://facelessuser.github.io/pymdown-extensions/extensions/tabbed/#linked-tabs

const savedCodeTab = localStorage.getItem('savedTab')
const codeTabs = document.querySelectorAll(".tabbed-set > input")
for (const tab of codeTabs) {
    tab.addEventListener("click", () => {
        const current = document.querySelector(`label[for=${tab.id}]`)
        const pos = current.getBoundingClientRect().top
        const labelContent = current.innerHTML
        const labels = document.querySelectorAll('.tabbed-set > label, .tabbed-alternate > .tabbed-labels > label')
        for (const label of labels) {
            if (label.innerHTML === labelContent) {
                document.querySelector(`input[id=${label.getAttribute('for')}]`).checked = true
            }
        }

        // Preserve scroll position
        const delta = (current.getBoundingClientRect().top) - pos
        window.scrollBy(0, delta)

        // Save
        localStorage.setItem('savedTab', labelContent)
    })

    // Select saved tab
    const current = document.querySelector(`label[for=${tab.id}]`)
    const labelContent = current.innerHTML
    if (savedCodeTab === labelContent) {
        tab.checked = true
    }
}

// Lightbox for screenshot

const lightbox = document.createElement('div');
lightbox.classList.add('lightbox');
document.body.appendChild(lightbox);

const showScreenshotOverlay = (e, el, group, index) => {
    lightbox.classList.add('show');
    document.addEventListener('keydown', nextScreenshotKeyboardListener);
    return showScreenshot(e, group, index);
};

const showScreenshot = (e, group, index) => {
    const actualIndex = resolveScreenshotIndex(group, index);
    lightbox.innerHTML = '<div class="close-lightbox"></div>' + screenshots[group][actualIndex].innerHTML;
    lightbox.querySelector('img').onclick = (e) => { return showScreenshot(e, group, actualIndex+1); };
    currentScreenshotGroup = group;
    currentScreenshotIndex = actualIndex;
    e.stopPropagation();
    return false;
};

const nextScreenshot = (e) => {
    return showScreenshot(e, currentScreenshotGroup, currentScreenshotIndex+1);
};

const previousScreenshot = (e) => {
    return showScreenshot(e, currentScreenshotGroup, currentScreenshotIndex-1);
};

const resolveScreenshotIndex = (group, index) => {
    if (index < 0) {
        return screenshots[group].length - 1;
    } else if (index > screenshots[group].length - 1) {
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

let currentScreenshotGroup = '';
let currentScreenshotIndex = 0;
let screenshots = {};
Array.from(document.getElementsByClassName('screenshots')).forEach((sg) => {
    const group = sg.id;
    screenshots[group] = [...sg.querySelectorAll('a')];
    screenshots[group].forEach((el, index) => {
        el.onclick = (e) => { return showScreenshotOverlay(e, el, group, index); };
    });
});

lightbox.onclick = hideScreenshotOverlay;
