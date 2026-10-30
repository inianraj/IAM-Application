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
    h("p", { class: "muted" }, "Enter the email and password of your account."),
    field("Email", { type: "email", name: "email", required: true, autocomplete: "email" }),
    field("Password", { type: "password", name: "password", required: true, autocomplete: "current-password" }),
    h("button", { type: "submit", class: "primary" }, "Log in"),
  );

  onSubmit(form, async (data) => {
    const user = await post("/auth/login", { email: data.get("email"), password: data.get("password") }, { keepOn401: true });
    await ctx.signedIn(user);
  });

  container.append(form);
}

export async function createOrganisationView(container, ctx) {
  const form = h(
    "form",
    { class: "card" },
    h("h2", null, "Create an organisation"),
    h("p", { class: "muted" }, "The owner is created together with the organisation. Use a work email address."),
    field("Organisation name", { name: "name", required: true, maxlength: 100 }),
    field("Owner name", { name: "admin_name", required: true, maxlength: 100 }),
    field("Owner email", { type: "email", name: "admin_email", required: true }),
    field("Owner password (8 to 72 characters)", { type: "password", name: "admin_password", required: true, minlength: 8, maxlength: 72, autocomplete: "new-password" }),
    h("button", { type: "submit", class: "primary" }, "Create organisation"),
  );

  onSubmit(form, async (data) => {
    const created = await post("/auth/organisations", {
      name: data.get("name"),
      admin_name: data.get("admin_name"),
      admin_email: data.get("admin_email"),
      admin_password: data.get("admin_password"),
    });
    ctx.navigate("/login", {
      text: `Organisation "${created.name}" created (id ${created.id}). Log in as ${created.admin.email}.`,
    });
  });

  container.append(form);
}
