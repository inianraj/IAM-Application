import { h, clear, notify, clearNotice } from "./dom.js";
import { ApiError, setUnauthorizedHandler, post } from "./api.js";
import { state, can, restore, forget, signIn } from "./state.js";
import { loginView, createOrganisationView } from "./views/auth.js";
import { recordsView } from "./views/records.js";
import { groupsView } from "./views/groups.js";
import { peopleView } from "./views/people.js";
import { organisationView } from "./views/organisation.js";

// Screens for visitors who are not logged in.
const publicRoutes = {
  "/login": loginView,
  "/create-organisation": createOrganisationView,
};

// Screens for logged-in users. show() decides from the user's permissions
// whether the tab appears; the API still enforces the real rules.
const privateRoutes = {
  "/records": { label: "Records", view: recordsView, show: () => true },
  "/groups": { label: "Groups", view: groupsView, show: () => can("group:read") },
  "/people": { label: "People", view: peopleView, show: () => can("group:read") },
  "/organisation": { label: "Organisation", view: organisationView, show: () => can("organisation:read") },
};

let routeRun = 0; // numbers each render so a slow, stale one is thrown away
let pendingNotice = null;

function currentPath() {
  return location.hash.replace(/^#/, "") || "/";
}

// navigate("/login", { text: "Done" }) opens a screen, optionally with a message
// (a plain notify() would be cleared as the new screen opens).
function navigate(path, notice) {
  if (notice) pendingNotice = notice;

  if (currentPath() === path) {
    route();
  } else {
    location.hash = path;
  }
}

// What every screen can use to move around or change who is logged in.
const ctx = {
  navigate,
  async signedIn(user) {
    await signIn(user);
    navigate("/records");
  },
};

function renderChrome() {
  const nav = document.getElementById("nav");
  const account = document.getElementById("account");
  clear(nav);
  clear(account);

  if (state.user) {
    for (const [path, entry] of Object.entries(privateRoutes)) {
      if (!entry.show()) continue;
      nav.append(h("a", { href: `#${path}`, class: currentPath() === path ? "active" : "" }, entry.label));
    }
    account.append(
      h("span", { class: "muted" }, state.user.name),
      h("button", { class: "link", onclick: logout }, "Log out"),
    );
  } else {
    for (const [path, label] of [["/login", "Log in"], ["/create-organisation", "Create organisation"]]) {
      nav.append(h("a", { href: `#${path}`, class: currentPath() === path ? "active" : "" }, label));
    }
  }
}

function showError(container, err) {
  let text = err.message;
  if (err instanceof ApiError && err.status === 403) text = "You do not have permission to see this.";
  container.replaceChildren(h("div", { class: "notice error" }, text));
}

async function route() {
  const run = ++routeRun;
  clearNotice();
  if (pendingNotice) {
    notify(pendingNotice.text, pendingNotice.isError);
    pendingNotice = null;
  }

  const path = currentPath();
  let view;

  if (state.user) {
    const entry = privateRoutes[path];
    if (!entry || !entry.show()) return navigate("/records");
    view = entry.view;
  } else {
    view = publicRoutes[path];
    if (!view) return navigate("/login");
  }

  renderChrome();

  const container = document.getElementById("view");
  container.replaceChildren(h("p", { class: "muted" }, "Loading..."));

  // Build the screen off-page and swap it in only if it is still the current one.
  const section = h("section");
  try {
    await view(section, ctx);
  } catch (err) {
    if (run !== routeRun) return;
    return showError(container, err);
  }
  if (run === routeRun) container.replaceChildren(section);
}

async function logout() {
  try {
    await post("/auth/logout");
  } catch {
    // the session may already be gone; log out here anyway
  }
  forget();
  pendingNotice = { text: "You are logged out." };
  navigate("/login");
}

// A 401 anywhere means the session ended (it expired or the user was removed).
setUnauthorizedHandler(() => {
  if (!state.user) return;
  forget();
  pendingNotice = { text: "Your session has ended. Please log in again.", isError: true };
  navigate("/login");
});

window.addEventListener("hashchange", route);

await restore();
route();
