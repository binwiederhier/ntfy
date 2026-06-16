// Topic name generator for the ntfy docs
//
// A tiny helper that lives on the "Publishing" page. The user types a memorable
// prefix (e.g. "backups"), and the widget appends a random, hard-to-guess suffix
// (e.g. "backups-x7Kp2mQ9"). The result is a valid, unguessable topic name.
//
// Topic names on the server must match ^[-_A-Za-z0-9]{1,64}$ (see server.go), so as
// the user types we strip anything that isn't allowed (spaces, slashes, punctuation,
// emoji, ...) live and cap the whole thing at 64 characters. The random suffix is
// generated once on load and can be re-rolled with the "Regenerate suffix" button.
(function () {
  // Allowed topic characters per the server regex ^[-_A-Za-z0-9]{1,64}$
  const ALLOWED = /[^-_A-Za-z0-9]/g;
  const MAX_LEN = 64;

  // Suffix alphabet: full base62 (letters + digits). We deliberately keep look-alikes
  // (0/O, l/1) for maximum entropy -- this is a generated suffix, not something typed by
  // hand. Hyphen/underscore are excluded so the "-" separator stays visually clear.
  const SUFFIX_ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
  const SUFFIX_LEN = 10;

  // randomSuffix returns a cryptographically random string from SUFFIX_ALPHABET.
  // It uses rejection sampling to avoid the modulo bias that a plain `byte % 62` would
  // introduce (256 is not a multiple of 62), keeping every character equally likely.
  function randomSuffix() {
    const n = SUFFIX_ALPHABET.length;
    const limit = Math.floor(256 / n) * n; // largest multiple of n that fits in a byte
    const buf = new Uint8Array(1);
    let out = "";
    while (out.length < SUFFIX_LEN) {
      crypto.getRandomValues(buf);
      if (buf[0] < limit) {
        out += SUFFIX_ALPHABET[buf[0] % n];
      }
    }
    return out;
  }

  // sanitize strips everything that isn't a valid topic character.
  function sanitize(value) {
    return value.replace(ALLOWED, "");
  }

  function initTopicGenerator() {
    const root = document.getElementById("tg-widget");
    if (!root) return;

    const input = root.querySelector("#tg-input");
    const outputName = root.querySelector("#tg-output-name");
    const outputUrl = root.querySelector("#tg-output-url");
    const reroll = root.querySelector("#tg-reroll");

    let suffix = randomSuffix();

    // update recomputes the live preview from the (sanitized) input + current suffix.
    function update() {
      // Sanitize in place so the user sees disallowed characters disappear as they type.
      const cleaned = sanitize(input.value);
      if (cleaned !== input.value) {
        const pos = input.selectionStart - (input.value.length - cleaned.length);
        // Reassigning .value and setSelectionRange make the browser scroll the field into
        // view (there is no preventScroll option for setSelectionRange), which jumps the
        // whole page. Capture the scroll position and restore it afterwards.
        const scrollX = window.scrollX;
        const scrollY = window.scrollY;
        input.value = cleaned;
        // Best-effort caret restore so removing a bad char doesn't jump the cursor to the end.
        try { input.setSelectionRange(pos, pos); } catch { /* ignore */ }
        window.scrollTo(scrollX, scrollY);
      }

      // Compose "<prefix>-<suffix>", capped at the 64-char topic limit. With no prefix,
      // fall back to just the random suffix so the output is always a valid topic.
      let topic;
      if (cleaned === "") {
        topic = suffix;
      } else {
        const maxPrefix = MAX_LEN - suffix.length - 1; // room for "-" + suffix
        const prefix = cleaned.slice(0, Math.max(0, maxPrefix));
        topic = prefix === "" ? suffix : prefix + "-" + suffix;
      }

      outputName.textContent = topic;
      outputUrl.textContent = "https://ntfy.sh/" + topic;
    }

    input.addEventListener("input", update);
    reroll.addEventListener("click", function () {
      suffix = randomSuffix();
      update();
      input.focus();
    });

    // Copy buttons: copy the target output and briefly swap the clipboard icon for a checkmark,
    // mirroring the config generator's copy button behavior.
    const copyIcon = "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"14\" height=\"14\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"2\" stroke-linecap=\"round\" stroke-linejoin=\"round\"><rect x=\"9\" y=\"9\" width=\"13\" height=\"13\" rx=\"2\" ry=\"2\"></rect><path d=\"M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1\"></path></svg>";
    const checkIcon = "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"14\" height=\"14\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"2\" stroke-linecap=\"round\" stroke-linejoin=\"round\"><polyline points=\"20 6 9 17 4 12\"></polyline></svg>";
    root.querySelectorAll(".tg-btn-copy").forEach(function (btn) {
      btn.addEventListener("click", function () {
        const target = root.querySelector("#" + btn.dataset.copy);
        if (!target || !target.textContent) return;
        navigator.clipboard.writeText(target.textContent).then(function () {
          btn.innerHTML = checkIcon;
          btn.style.color = "var(--md-primary-fg-color)";
          setTimeout(function () {
            btn.innerHTML = copyIcon;
            btn.style.color = "";
          }, 2000);
        });
      });
    });

    update();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initTopicGenerator);
  } else {
    initTopicGenerator();
  }
})();
