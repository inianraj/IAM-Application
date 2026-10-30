import { get } from "./api.js";

// Who is logged in and what they may do. The session itself is an HttpOnly
// cookie the page can't read, so after a refresh the page asks the API again
// (GET /users/{id}: a user can always read themselves).
const STORAGE_KEY = "iam.userId";

export const state = {
  user: null,
  permissions: new Set(),
};

// can("record:read:org"): does the user hold this permission, through their own
// roles or their group's? This only
// decides what to show; the API enforces the real rules and answers 403.
export function can(permission) {
  return state.permissions.has(permission);
}

// canOn("record", "update", ownerId): the own/org pair of a scoped permission.
export function canOn(resource, action, ownerId) {
  if (can(`${resource}:${action}:org`)) return true;
  return ownerId === state.user.id && can(`${resource}:${action}:own`);
}

export function rememberedUserId() {
  try {
    return Number(localStorage.getItem(STORAGE_KEY)) || null;
  } catch {
    return null;
  }
}

export function forget() {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // storage unavailable: nothing to forget
  }
  state.user = null;
  state.permissions = new Set();
}

// signIn stores the user and loads their permissions.
export async function signIn(user) {
  state.user = user;
  try {
    localStorage.setItem(STORAGE_KEY, String(user.id));
  } catch {
    // storage unavailable: the user will log in again after a refresh
  }
  await loadPermissions();
}

export async function loadPermissions() {
  state.permissions = new Set();
  if (!state.user.organisation_id) return; // a legacy user without an organisation has none

  try {
    // The union of the user's own roles and the roles of their group.
    const response = await get(`/users/${state.user.id}/permissions`);
    state.permissions = new Set(response.permissions);
  } catch {
    // show the basics only
  }
}

// restore brings the session back after a page load. It returns true when the
// remembered user still has a valid session.
export async function restore() {
  const id = rememberedUserId();
  if (!id) return false;

  try {
    await signIn(await get(`/users/${id}`, { keepOn401: true }));
    return true;
  } catch {
    forget();
    return false;
  }
}
