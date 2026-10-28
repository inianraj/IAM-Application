import { h, field, notify } from "../dom.js";
import { get, post, put, del } from "../api.js";
import { state, can, canOn } from "../state.js";

// "My records", and with record:read:org also every record of the organisation.
export async function recordsView(container) {
  const wholeOrganisation = can("record:read:org");
  let showAll = false;

  const title = h("h2");
  const list = h("div");

  const toggle = wholeOrganisation
    ? h(
        "label",
        { class: "inline" },
        h("input", {
          type: "checkbox",
          onchange: (event) => {
            showAll = event.target.checked;
            load();
          },
        }),
        " Show every record in the organisation",
      )
    : null;

  container.append(h("div", { class: "row" }, title, toggle), createForm(), list);

  function createForm() {
    if (!can("record:create:own")) return null;

    const form = h(
      "form",
      { class: "card" },
      h("h3", null, "New record"),
      field("Title", { name: "title", required: true, maxlength: 200 }),
      field("Description", { name: "description" }),
      h("button", { type: "submit", class: "primary" }, "Add record"),
    );

    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      const data = new FormData(form);
      try {
        await post("/records", { user_id: state.user.id, title: data.get("title"), description: data.get("description") });
        form.reset();
        notify("Record added.");
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    });

    return form;
  }

  async function load() {
    title.textContent = showAll ? "Records in the organisation" : "My records";
    try {
      const records = await get(showAll ? "/records?scope=org" : "/records");
      renderList(records);
    } catch (err) {
      list.replaceChildren(h("div", { class: "notice error" }, err.message));
    }
  }

  function renderList(records) {
    if (records.length === 0) {
      list.replaceChildren(h("p", { class: "muted" }, "No records yet."));
      return;
    }

    const rows = records.map((record) => recordRow(record));
    list.replaceChildren(
      h(
        "table",
        null,
        h("thead", null, h("tr", null, ["Id", "Owner", "Title", "Description", "Updated", ""].map((c) => h("th", null, c)))),
        h("tbody", null, rows),
      ),
    );
  }

  function recordRow(record) {
    const row = h("tr");

    const showRow = () => {
      row.replaceChildren(
        h("td", null, record.id),
        h("td", null, record.user_id === state.user.id ? "me" : `user ${record.user_id}`),
        h("td", null, record.title),
        h("td", null, record.description),
        h("td", { class: "muted" }, new Date(record.updated_at).toLocaleString()),
        h(
          "td",
          { class: "actions" },
          canOn("record", "update", record.user_id) ? h("button", { onclick: editRow }, "Edit") : null,
          canOn("record", "delete", record.user_id) ? h("button", { class: "danger", onclick: remove }, "Delete") : null,
        ),
      );
    };

    const editRow = () => {
      const titleInput = h("input", { value: record.title, maxlength: 200 });
      const descriptionInput = h("input", { value: record.description });

      row.replaceChildren(
        h("td", null, record.id),
        h("td", null, record.user_id === state.user.id ? "me" : `user ${record.user_id}`),
        h("td", null, titleInput),
        h("td", null, descriptionInput),
        h("td"),
        h(
          "td",
          { class: "actions" },
          h(
            "button",
            {
              class: "primary",
              onclick: async () => {
                try {
                  Object.assign(record, await put(`/records/${record.id}`, { title: titleInput.value, description: descriptionInput.value }));
                  notify("Record saved.");
                  showRow();
                } catch (err) {
                  notify(err.message, true);
                }
              },
            },
            "Save",
          ),
          h("button", { onclick: showRow }, "Cancel"),
        ),
      );
    };

    const remove = async () => {
      if (!confirm(`Delete the record "${record.title}"?`)) return;
      try {
        await del(`/records/${record.id}`);
        notify("Record deleted.");
        await load();
      } catch (err) {
        notify(err.message, true);
      }
    };

    showRow();
    return row;
  }

  await load();
}
