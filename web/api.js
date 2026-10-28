import { API_BASE } from "./config.js";

// ApiError carries the HTTP status and the API's { "error": "..." } message.
export class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

let onUnauthorized = () => {};

// The app registers what happens when the session is gone (a 401).
export function setUnauthorizedHandler(fn) {
  onUnauthorized = fn;
}

// request sends JSON to the API with the session cookie. It returns the parsed
// JSON (null for 204) and throws an ApiError for any other status. A 401 also
// calls the unauthorized handler, unless options.keepOn401 is set (the login
// form uses that: a wrong email is not an expired session).
export async function request(method, path, body, options = {}) {
  let response;
  try {
    response = await fetch(API_BASE + path, {
      method,
      credentials: "include",
      headers: body === undefined ? {} : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError(0, "Cannot reach the server. Is the API running, and is this page's origin in ALLOWED_ORIGINS?");
  }

  if (response.status === 204) return null;

  let data = null;
  try {
    data = await response.json();
  } catch {
    // not JSON; fall through to the status message below
  }

  if (!response.ok) {
    if (response.status === 401 && !options.keepOn401) onUnauthorized();
    throw new ApiError(response.status, (data && data.error) || `Request failed (${response.status})`);
  }

  return data;
}

export const get = (path, options) => request("GET", path, undefined, options);
export const post = (path, body, options) => request("POST", path, body ?? {}, options);
export const put = (path, body, options) => request("PUT", path, body, options);
export const del = (path, body, options) => request("DELETE", path, body, options);
