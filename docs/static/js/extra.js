// Link tabs, as per https://facelessuser.github.io/pymdown-extensions/extensions/tabbed/#linked-tabs

const savedTab = localStorage.getItem('savedTab')
const tabs = document.querySelectorAll(".tabbed-set > input")
for (const tab of tabs) {
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
    if (savedTab === labelContent) {
        tab.checked = true
    }
}
