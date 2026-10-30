import { h, field, notify } from "../dom.js";
import { get, post, put, del } from "../api.js";
import { state, can, canOn, forget } from "../state.js";
import { rolePicker } from "../pickers.js";

// Every user of the organisation, with the actions the signed-in user's
// permissions allow: set a group, choose the roles they hold directly,
// rename, delete.
export async function peopleView(container, ctx) {
  const organisationId = state.user.organisation_id;
  const canSeeRoles = can("role:read");
  const canAssignRoles = canSeeRoles && can("role:assign");

  // Groups and roles are looked up once, to show names and to fill the pickers.
  const groups = can("group:read") ? await get(`/groups?organisation_id=${organisationId}`) : [];
  const roles = canSeeRoles ? await get("/roles") : [];
  const list = h("div");

  container.append(h("h2", null, "People"), addMemberForm(), list);

  function addMemberForm() {
    // POST /auth/users needs user:assign_group (admins); the user joins the caller's organisation.
    if (!can("user:assign_group")) return null;

    const picker = canAssignRoles ? rolePicker(roles, new Set()) : null;
    const form = h(
      "form",
      { class: "card" },
      h("h3", null, "Add a user"),
      h("p", { class: "muted" }, "The new user joins your organisation with no group. Without roles chosen below they get the member role."),
      field("Name", { name: "name", required: true, maxlength: 100 }),
      field("Email", { type: "email", name: "email", required: true }),
      field("Password (8 to 72 characters)", { type: "password", name: "password", required: true, minlength: 8, maxlength: 72, autocomplete: "new-password" }),
      picker ? h("p", { class: "muted" }, "Roles (optional):") : null,
      picker ? picker.element : null,
      h("button", { type: "submit", class: "primary" }, "Add user"),
    );

    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const data = new FormData(form);
      const body = { name: data.get("name"), email: data.get("email"), password: data.get("password") };
      if (picker && picker.selected().length > 0) body.role_ids = picker.selected();

      try {
        await post("/auth/users", body);
        notify("User added.");
        form.reset();
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    });

    return form;
  }

  async function load() {
    const users = await get("/users");
    // The roles each user holds directly, when the signed-in user may see them.
    const direct = await Promise.all(users.map((u) => (canSeeRoles ? get(`/users/${u.id}/roles`) : [])));

    list.replaceChildren(
      users.length === 0
        ? h("p", { class: "muted" }, "No users.")
        : h(
            "table",
            null,
            h("thead", null, h("tr", null, ["Id", "Name", "Email", "Group", "Roles", ""].map((c) => h("th", null, c)))),
            h("tbody", null, users.map((user, i) => userRow(user, direct[i]))),
          ),
    );
  }

  function userRow(user, userRoles) {
    const isMe = user.id === state.user.id;
    const group = groups.find((g) => g.id === user.group_id);

    const select = h(
      "select",
      null,
      h("option", { value: 0, selected: !user.group_id }, "(no group)"),
      groups.map((g) => h("option", { value: g.id, selected: g.id === user.group_id }, g.name)),
    );

    const actions = [];
    if (can("user:assign_group") && groups.length > 0) {
      actions.push(select, h("button", { onclick: move }, "Move"));
    }
    if (canAssignRoles) actions.push(h("button", { onclick: editRoles }, "Roles"));
    if (canOn("user", "update", user.id)) actions.push(h("button", { onclick: rename }, "Rename"));
    if (can("user:delete")) actions.push(h("button", { class: "danger", onclick: remove }, "Delete"));

    async function move() {
      const groupId = Number(select.value);
      if (groupId === (user.group_id || 0)) return;
      try {
        await put(`/users/${user.id}/group`, { group_id: groupId });
        notify(`${user.name} moved. Their permissions change on their next request.`);
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    }

    async function editRoles() {
      const row = rolesRow;
      if (row.hidden === false) {
        row.hidden = true;
        return;
      }

      const picker = rolePicker(roles, new Set(userRoles.map((r) => r.id)));
      row.firstChild.replaceChildren(
        h("p", { class: "muted" }, `Roles ${user.name} holds directly (their group may give more):`),
        picker.element,
        h(
          "button",
          {
            class: "primary",
            onclick: async () => {
              try {
                await put(`/users/${user.id}/roles`, { role_ids: picker.selected() });
                notify(`Roles of ${user.name} saved. They apply on their next request.`);
                await load();
              } catch (err) {
                notify(err.message, true);
              }
            },
          },
          "Save roles",
        ),
      );
      row.hidden = false;
    }

    async function rename() {
      const name = prompt("New name:", user.name);
      if (name === null || name.trim() === "" || name === user.name) return;
      try {
        await put(`/users/${user.id}`, { name });
        notify("Name changed.");
        if (isMe) state.user.name = name;
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    }

    async function remove() {
      const warning = isMe ? "This deletes YOUR account." : `Delete ${user.name}?`;
      if (!confirm(`${warning} This cannot be undone.`)) return;
      try {
        await del(`/users/${user.id}`);
        if (isMe) {
          forget();
          ctx.navigate("/login", { text: "Your account was deleted." });
          return;
        }
        notify("User deleted.");
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    }

    // Expanded under the row by the Roles button.
    const rolesRow = h("tr", { hidden: true }, h("td", { colspan: 6 }));

    // A table row can't hold a second row, so both are returned as a fragment.
    const fragment = document.createDocumentFragment();
    fragment.append(
      h(
        "tr",
        null,
        h("td", null, user.id),
        h("td", null, user.name, isMe ? " (you)" : ""),
        h("td", null, user.email),
        h("td", null, group ? group.name : ""),
        h("td", null, userRoles.map((r) => r.name).join(", ")),
        h("td", { class: "actions" }, actions),
      ),
      rolesRow,
    );
    return fragment;
  }

  await load();
}
