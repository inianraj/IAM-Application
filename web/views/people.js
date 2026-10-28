import { h, field, notify } from "../dom.js";
import { get, post, put, del } from "../api.js";
import { state, can, canOn, forget } from "../state.js";

// The members of every group of the organisation, with the actions the
// user's permissions allow.
export async function peopleView(container, ctx) {
  const organisationId = state.user.organisation_id;
  const list = h("div");

  container.append(h("h2", null, "People"), addMemberForm(), list);

  function addMemberForm() {
    // POST /auth/users needs user:assign_group (admins); the user joins the caller's organisation.
    if (!can("user:assign_group")) return null;

    const form = h(
      "form",
      { class: "card" },
      h("h3", null, "Add a member"),
      h("p", { class: "muted" }, "The new user joins your organisation in the member group."),
      field("Name", { name: "name", required: true, maxlength: 100 }),
      field("Email", { type: "email", name: "email", required: true }),
      h("button", { type: "submit", class: "primary" }, "Add member"),
    );

    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const data = new FormData(form);
      try {
        await post("/auth/users", { name: data.get("name"), email: data.get("email") });
        notify("Member added.");
        form.reset();
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    });

    return form;
  }

  async function load() {
    const groups = await get(`/groups?organisation_id=${organisationId}`);
    const memberLists = await Promise.all(groups.map((group) => get(`/groups/${group.id}/users`)));

    const rows = [];
    groups.forEach((group, index) => {
      for (const member of memberLists[index]) rows.push(memberRow(member, group, groups));
    });

    list.replaceChildren(
      rows.length === 0
        ? h("p", { class: "muted" }, "No members.")
        : h(
            "table",
            null,
            h("thead", null, h("tr", null, ["Id", "Name", "Email", "Group", ""].map((c) => h("th", null, c)))),
            h("tbody", null, rows),
          ),
    );
  }

  function memberRow(member, group, groups) {
    const isMe = member.id === state.user.id;

    const select = h(
      "select",
      null,
      groups.map((g) => h("option", { value: g.id, selected: g.id === group.id }, g.name)),
    );

    const actions = [];
    if (can("user:assign_group")) {
      actions.push(select, h("button", { onclick: move }, "Move"));
    }
    if (canOn("user", "update", member.id)) actions.push(h("button", { onclick: rename }, "Rename"));
    if (can("user:delete")) actions.push(h("button", { class: "danger", onclick: remove }, "Delete"));

    async function move() {
      const groupId = Number(select.value);
      if (groupId === group.id) return;
      try {
        await put(`/users/${member.id}/group`, { group_id: groupId });
        notify(`${member.name} moved. Their permissions change on their next request.`);
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    }

    async function rename() {
      const name = prompt("New name:", member.name);
      if (name === null || name.trim() === "" || name === member.name) return;
      try {
        await put(`/users/${member.id}`, { name });
        notify("Name changed.");
        if (isMe) state.user.name = name;
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    }

    async function remove() {
      const warning = isMe ? "This deletes YOUR account." : `Delete ${member.name}?`;
      if (!confirm(`${warning} This cannot be undone.`)) return;
      try {
        await del(`/users/${member.id}`);
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

    return h(
      "tr",
      null,
      h("td", null, member.id),
      h("td", null, member.name, isMe ? " (you)" : ""),
      h("td", null, member.email),
      h("td", null, group.name),
      h("td", { class: "actions" }, actions),
    );
  }

  await load();
}
