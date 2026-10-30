import { h, field, notify } from "../dom.js";
import { get, post, put, del } from "../api.js";
import { can } from "../state.js";
import { permissionPicker } from "../pickers.js";

const BUILTIN = ["owner", "admin", "member", "guest"];

// A role is a named set of permissions. Users hold roles directly (People) and
// through their group (Groups).
export async function rolesView(container) {
  const catalogue = await get("/permissions");
  const list = h("div");

  container.append(h("h2", null, "Roles"), createForm(), list);

  async function load() {
    const roles = await get("/roles");
    list.replaceChildren(...roles.map((role) => roleCard(role)));
  }

  function createForm() {
    if (!can("role:create")) return null;

    const nameInput = field("Role name", { name: "name", required: true, maxlength: 50, placeholder: "e.g. auditor" });
    const picker = permissionPicker(catalogue, new Set());
    const form = h(
      "form",
      { class: "card" },
      h("h3", null, "New role"),
      nameInput,
      h("p", { class: "muted" }, "Choose what the role allows. You can only pick permissions you have yourself:"),
      picker.element,
      h("button", { type: "submit", class: "primary" }, "Create role"),
    );

    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      try {
        await post("/roles", { name: new FormData(form).get("name"), permissions: picker.selected() });
        notify("Role created.");
        form.reset();
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    });

    return form;
  }

  function roleCard(role) {
    const builtin = BUILTIN.includes(role.name);
    const details = h("div");

    const card = h(
      "div",
      { class: "card" },
      h(
        "div",
        { class: "row" },
        h("h3", null, role.name, " ", builtin ? h("span", { class: "badge" }, "built-in") : null),
        h(
          "div",
          { class: "actions" },
          h("button", { onclick: () => togglePermissions() }, "Permissions"),
          can("role:update") && !builtin ? h("button", { onclick: rename }, "Rename") : null,
          can("role:delete") && !builtin ? h("button", { class: "danger", onclick: remove }, "Delete") : null,
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
        const current = new Set((await get(`/roles/${role.id}/permissions`)).permissions);

        if (can("role:update") && !builtin) {
          const picker = permissionPicker(catalogue, current);
          details.replaceChildren(
            picker.element,
            h(
              "button",
              {
                class: "primary",
                onclick: async () => {
                  try {
                    await put(`/roles/${role.id}/permissions`, { permissions: picker.selected() });
                    notify(`Permissions of "${role.name}" saved. They apply on the next request of everyone who has the role.`);
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
            builtin ? h("p", { class: "muted" }, "Built-in roles cannot be changed.") : null,
          );
        }
      } catch (err) {
        details.replaceChildren(h("div", { class: "notice error" }, err.message));
      }
    }

    async function rename() {
      const name = prompt("New name for the role:", role.name);
      if (name === null || name.trim() === "" || name === role.name) return;
      try {
        await put(`/roles/${role.id}`, { name });
        notify("Role renamed.");
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    }

    async function remove() {
      if (!confirm(`Delete the role "${role.name}"?`)) return;
      try {
        await del(`/roles/${role.id}`);
        notify("Role deleted.");
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    }

    return card;
  }

  await load();
}
