import { h, field, notify } from "../dom.js";
import { post } from "../api.js";

// Runs fn when the form is submitted, disabling the button meanwhile and
// showing any API error as a message.
function onSubmit(form, fn) {
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const button = form.querySelector("button[type=submit]");
    button.disabled = true;
    try {
      await fn(new FormData(form));
    } catch (err) {
      notify(err.message, true);
    } finally {
      button.disabled = false;
    }
  });
  return form;
}

export async function loginView(container, ctx) {
  const form = h(
    "form",
    { class: "card" },
    h("h2", null, "Log in"),
    h("p", { class: "muted" }, "Enter the email of your account. (Development setup: there is no password.)"),
    field("Email", { type: "email", name: "email", required: true, autocomplete: "email" }),
    h("button", { type: "submit", class: "primary" }, "Log in"),
  );

  onSubmit(form, async (data) => {
    const user = await post("/auth/login", { email: data.get("email") }, { keepOn401: true });
    await ctx.signedIn(user);
  });

  container.append(form);
}

export async function createOrganisationView(container, ctx) {
  const form = h(
    "form",
    { class: "card" },
    h("h2", null, "Create an organisation"),
    h("p", { class: "muted" }, "The administrator is created together with the organisation. Use a work email address."),
    field("Organisation name", { name: "name", required: true, maxlength: 100 }),
    field("Administrator name", { name: "admin_name", required: true, maxlength: 100 }),
    field("Administrator email", { type: "email", name: "admin_email", required: true }),
    h("button", { type: "submit", class: "primary" }, "Create organisation"),
  );

  onSubmit(form, async (data) => {
    const created = await post("/auth/organisations", {
      name: data.get("name"),
      admin_name: data.get("admin_name"),
      admin_email: data.get("admin_email"),
    });
    ctx.navigate("/login", {
      text: `Organisation "${created.name}" created (id ${created.id}). Log in as ${created.admin.email}.`,
    });
  });

  container.append(form);
}
