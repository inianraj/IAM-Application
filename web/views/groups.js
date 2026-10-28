import { h, field, notify } from "../dom.js";
import { get, post, put, del } from "../api.js";
import { state, can } from "../state.js";

const BUILTIN = ["admin", "member"];

// A list of checkboxes for the permissions, grouped by resource. selected() returns
// the checked keys.
function permissionPicker(catalogue, selectedKeys) {
  const boxes = new Map();
  const resources = new Map();

  for (const permission of catalogue) {
    if (!resources.has(permission.resource)) resources.set(permission.resource, []);
    resources.get(permission.resource).push(permission);
  }

  const element = h("div", { class: "permissions" });
  for (const [resource, permissions] of resources) {
    element.append(
      h(
        "fieldset",
        null,
        h("legend", null, resource),
        permissions.map((permission) => {
          const box = h("input", { type: "checkbox", checked: selectedKeys.has(permission.key) });
          boxes.set(permission.key, box);
          return h("label", { class: "inline" }, box, " ", h("code", null, permission.key), " ", h("span", { class: "muted" }, permission.description));
        }),
      ),
    );
  }

  return { element, selected: () => [...boxes].filter(([, box]) => box.checked).map(([key]) => key) };
}

export async function groupsView(container) {
  const organisationId = state.user.organisation_id;
  const canManage = can("group:manage_permissions");

  // Only admins can read the catalogue (GET /permissions).
  const catalogue = canManage ? await get("/permissions") : [];
  const list = h("div");

  container.append(h("h2", null, "Groups"), createForm(), list);

  async function load() {
    const groups = await get(`/groups?organisation_id=${organisationId}`);
    list.replaceChildren(...groups.map((group) => groupCard(group)));
  }

  function createForm() {
    if (!can("group:create")) return null;

    const nameInput = field("Group name", { name: "name", required: true, maxlength: 50, placeholder: "e.g. auditors" });
    const picker = h("div");
    const form = h("form", { class: "card" }, h("h3", null, "New group"), nameInput, picker);

    if (canManage) {
      // Start from what the built-in member group may do.
      picker.append(h("p", { class: "muted" }, "Choose what the group may do:"));
      loadMemberDefaults().then((defaults) => {
        const chooser = permissionPicker(catalogue, defaults);
        picker.append(chooser.element);
        picker.chooser = chooser;
      });
    } else {
      picker.append(h("p", { class: "muted" }, "The group starts with the same permissions as members."));
    }

    form.append(h("button", { type: "submit", class: "primary" }, "Create group"));
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const body = { organisation_id: organisationId, name: new FormData(form).get("name") };
      if (canManage && picker.chooser) body.permissions = picker.chooser.selected();

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

  async function loadMemberDefaults() {
    try {
      const groups = await get(`/groups?organisation_id=${organisationId}`);
      const member = groups.find((g) => g.name === "member");
      if (member) return new Set((await get(`/groups/${member.id}/permissions`)).permissions);
    } catch {
      // start with nothing checked
    }
    return new Set();
  }

  function groupCard(group) {
    const builtin = BUILTIN.includes(group.name);
    const details = h("div");

    const card = h(
      "div",
      { class: "card" },
      h(
        "div",
        { class: "row" },
        h("h3", null, group.name, " ", builtin ? h("span", { class: "badge" }, "built-in") : null),
        h(
          "div",
          { class: "actions" },
          h("button", { onclick: () => togglePermissions() }, "Permissions"),
          can("group:update") && !builtin ? h("button", { onclick: rename }, "Rename") : null,
          can("group:delete") && !builtin ? h("button", { class: "danger", onclick: remove }, "Delete") : null,
        ),
      ),
      details,
    );

    let open = false;
    async function togglePermissions() {
      open = !open;
      if (!open) {
        details.replaceChildren();
        return;
      }

      try {
        const current = new Set((await get(`/groups/${group.id}/permissions`)).permissions);

        if (canManage && !builtin) {
          const picker = permissionPicker(catalogue, current);
          details.replaceChildren(
            picker.element,
            h(
              "button",
              {
                class: "primary",
                onclick: async () => {
                  try {
                    await put(`/groups/${group.id}/permissions`, { permissions: picker.selected() });
                    notify(`Permissions of "${group.name}" saved. They apply on the members' next request.`);
                  } catch (err) {
                    notify(err.message, true);
                  }
                },
              },
              "Save permissions",
            ),
          );
        } else {
          details.replaceChildren(
            current.size === 0
              ? h("p", { class: "muted" }, "No permissions.")
              : h("ul", { class: "keys" }, [...current].sort().map((key) => h("li", null, h("code", null, key)))),
            builtin ? h("p", { class: "muted" }, "Built-in groups cannot be changed.") : null,
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
