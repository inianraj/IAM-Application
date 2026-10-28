// Tiny helpers for building the page. Server data is only ever added as text
// (never as HTML), so a name or title can't inject script into the page.

// h("button", { class: "primary", onclick: fn }, "Save") builds an element.
// Children can be strings, elements, arrays or null/false (ignored).
export function h(tag, props, ...children) {
  const el = document.createElement(tag);

  for (const [key, value] of Object.entries(props || {})) {
    if (value === null || value === undefined || value === false) continue;

    if (key === "class") {
      el.className = value;
    } else if (key.startsWith("on")) {
      el.addEventListener(key.slice(2), value);
    } else if (["value", "checked", "disabled", "selected", "hidden"].includes(key)) {
      el[key] = value;
    } else {
      el.setAttribute(key, value === true ? "" : value);
    }
  }

  for (const child of children.flat()) {
    if (child === null || child === undefined || child === false) continue;
    el.append(child instanceof Node ? child : String(child));
  }

  return el;
}

export function clear(el) {
  el.replaceChildren();
}

let messageTimer;

// Shows a short message at the top of the page.
export function notify(text, isError = false) {
  const box = document.getElementById("message");
  clearTimeout(messageTimer);

  box.replaceChildren(h("div", { class: isError ? "notice error" : "notice ok" }, text));
  if (!isError) {
    messageTimer = setTimeout(() => box.replaceChildren(), 5000);
  }
}

export function clearNotice() {
  clearTimeout(messageTimer);
  document.getElementById("message").replaceChildren();
}

// A labelled form field: field("Email", { type: "email", name: "email", required: true }).
export function field(label, attrs) {
  return h("label", { class: "field" }, h("span", null, label), h("input", attrs));
}
