import { h, field, notify } from "../dom.js";
import { get, post, put, del } from "../api.js";
import { state, can } from "../state.js";
import { rolePicker } from "../pickers.js";

// A group is a team. It is optional for a user, and gives its members the
// roles it has on top of the roles they hold themselves.
export async function groupsView(container) {
  const organisationId = state.user.organisation_id;
  const canAssign = can("role:assign") && can("role:read");

  // The roles a group can be given (GET /roles needs role:read).
  const roles = canAssign ? await get("/roles") : [];
  const list = h("div");

  container.append(h("h2", null, "Groups"), createForm(), list);

  async function load() {
    const groups = await get(`/groups?organisation_id=${organisationId}`);
    list.replaceChildren(...groups.map((group) => groupCard(group)));
  }

  function createForm() {
    if (!can("group:create")) return null;

    const nameInput = field("Group name", { name: "name", required: true, maxlength: 50, placeholder: "e.g. engineering" });
    const picker = canAssign ? rolePicker(roles, new Set()) : null;
    const form = h(
      "form",
      { class: "card" },
      h("h3", null, "New group"),
      nameInput,
      picker ? h("p", { class: "muted" }, "Roles the members of the group get:") : null,
      picker ? picker.element : null,
      h("button", { type: "submit", class: "primary" }, "Create group"),
    );

    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const body = { organisation_id: organisationId, name: new FormData(form).get("name") };
      if (picker) body.role_ids = picker.selected();

      try {
        await post("/groups", body);
        notify("Group created.");
        form.reset();
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    });

    return form;
  }

  function groupCard(group) {
    const details = h("div");

    const card = h(
      "div",
      { class: "card" },
      h(
        "div",
        { class: "row" },
        h("h3", null, group.name),
        h(
          "div",
          { class: "actions" },
          h("button", { onclick: () => toggleRoles() }, "Roles"),
          can("group:update") ? h("button", { onclick: rename }, "Rename") : null,
          can("group:delete") ? h("button", { class: "danger", onclick: remove }, "Delete") : null,
        ),
      ),
      details,
    );

    let open = false;
    async function toggleRoles() {
      open = !open;
      if (!open) {
        details.replaceChildren();
        return;
      }

      try {
        const current = await get(`/groups/${group.id}/roles`);

        if (canAssign) {
          const picker = rolePicker(roles, new Set(current.map((r) => r.id)));
          details.replaceChildren(
            picker.element,
            h(
              "button",
              {
                class: "primary",
                onclick: async () => {
                  try {
                    await put(`/groups/${group.id}/roles`, { role_ids: picker.selected() });
                    notify(`Roles of "${group.name}" saved. They apply on the members' next request.`);
                  } catch (err) {
                    notify(err.message, true);
                  }
                },
              },
              "Save roles",
            ),
          );
        } else {
          details.replaceChildren(
            current.length === 0
              ? h("p", { class: "muted" }, "No roles.")
              : h("ul", { class: "keys" }, current.map((r) => h("li", null, h("code", null, r.name)))),
          );
        }
      } catch (err) {
        details.replaceChildren(h("div", { class: "notice error" }, err.message));
      }
    }

    async function rename() {
      const name = prompt("New name for the group:", group.name);
      if (name === null || name.trim() === "" || name === group.name) return;
      try {
        await put(`/groups/${group.id}`, { organisation_id: organisationId, name });
        notify("Group renamed.");
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    }

    async function remove() {
      if (!confirm(`Delete the group "${group.name}"?`)) return;
      try {
        await del(`/groups/${group.id}`);
        notify("Group deleted.");
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    }

    return card;
  }

  await load();
}
