import { h } from "./dom.js";

// A list of checkboxes for the permissions, grouped by resource. selected()
// returns the checked keys.
export function permissionPicker(catalogue, selectedKeys) {
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

// A list of checkboxes for roles (objects from GET /roles). selected() returns
// the checked role ids.
export function rolePicker(roles, selectedIds) {
  const boxes = new Map();

  const element = h(
    "div",
    { class: "permissions" },
    roles.map((role) => {
      const box = h("input", { type: "checkbox", checked: selectedIds.has(role.id) });
      boxes.set(role.id, box);
      return h("label", { class: "inline" }, box, " ", role.name);
    }),
  );

  return { element, selected: () => [...boxes].filter(([, box]) => box.checked).map(([id]) => id) };
}
