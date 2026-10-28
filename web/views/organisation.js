import { h, field, notify } from "../dom.js";
import { get, put, del } from "../api.js";
import { can, forget } from "../state.js";

export async function organisationView(container, ctx) {
  const organisation = await get("/organisation");

  container.append(
    h("h2", null, "Organisation"),
    h("div", { class: "card" }, h("p", null, h("strong", null, organisation.name)), h("p", { class: "muted" }, `Id ${organisation.id}`)),
    renameForm(),
    deleteSection(),
  );

  function renameForm() {
    if (!can("organisation:update")) return null;

    const form = h(
      "form",
      { class: "card" },
      h("h3", null, "Rename"),
      field("New name", { name: "name", required: true, maxlength: 100, value: organisation.name }),
      h("button", { type: "submit", class: "primary" }, "Rename"),
    );

    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      try {
        const updated = await put("/organisation", { name: new FormData(form).get("name") });
        organisation.name = updated.name;
        notify("Organisation renamed.");
        ctx.navigate("/organisation");
      } catch (err) {
        notify(err.message, true);
      }
    });

    return form;
  }

  function deleteSection() {
    if (!can("organisation:delete")) return null;

    const confirmInput = h("input", { placeholder: organisation.name, autocomplete: "off" });
    const button = h("button", { class: "danger", disabled: true }, "Delete the organisation permanently");

    confirmInput.addEventListener("input", () => {
      button.disabled = confirmInput.value.trim() !== organisation.name;
    });

    button.addEventListener("click", async () => {
      button.disabled = true;
      try {
        await del("/organisation", { confirm_name: confirmInput.value });
        forget();
        ctx.navigate("/login", { text: `"${organisation.name}" and everything in it was deleted.` });
      } catch (err) {
        notify(err.message, true);
        button.disabled = false;
      }
    });

    return h(
      "div",
      { class: "card danger-zone" },
      h("h3", null, "Delete this organisation"),
      h("p", null, "This permanently deletes the organisation, all its groups, all its users (you included) and all their records. It cannot be undone."),
      h("label", { class: "field" }, h("span", null, `Type "${organisation.name}" to confirm`), confirmInput),
      button,
    );
  }
}
