// Drives the dashboard's htmx forms in Chromium and checks the save feedback
// each form shows. TestDashboardSaveFeedbackInBrowser runs it with the server
// URL as its argument and the Playwright module path in
// PERMITPAL_PLAYWRIGHT_MODULE.
const assert = require("node:assert/strict");
const { chromium } = require(process.env.PERMITPAL_PLAYWRIGHT_MODULE);

const baseURL = process.argv[2];

// countRequests counts sent requests in window.sent as they are sent, and
// finished ones in window.loaded, a task after every load listener the page
// added before sending, so htmx and the page have handled the response by
// the time it changes.
function countRequests() {
  window.sent = 0;
  window.loaded = 0;
  const send = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.send = function (...args) {
    window.sent++;
    this.addEventListener("loadend", () => setTimeout(() => window.loaded++));
    return send.apply(this, args);
  };
}

// settle runs action and waits until count more requests have been handled
// and htmx has settled any swapped content. htmx wires up swapped-in rows
// only when it settles them, and removes the classes below once it has.
async function settle(page, action, count = 1) {
  const loaded = await page.evaluate(() => window.loaded);
  await action();
  await page.waitForFunction(
    (want) => window.loaded >= want && !document.querySelector(".htmx-swapping, .htmx-settling, .htmx-added"),
    loaded + count,
    { timeout: 5000 },
  );
}

async function until(check) {
  for (let i = 0; !check(); i++) {
    if (i === 500) {
      throw new Error("timed out waiting for requests");
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
}

// Only visible alerts and statuses count: hidden ones are not feedback.
const alerts = (scope) => scope.getByRole("alert").allInnerTexts();
const statuses = (scope) => scope.getByRole("status").allInnerTexts();

(async () => {
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage();
    await page.addInitScript(countRequests);
    const problems = [];
    page.on("pageerror", (err) => problems.push(err.message));
    page.on("dialog", (dialog) => {
      problems.push("dialog: " + dialog.message());
      dialog.dismiss();
    });
    page.on("console", (msg) => {
      if (msg.text().includes("Content Security Policy")) {
        problems.push(msg.text());
      }
    });

    await page.goto(baseURL + "/login");
    await page.getByLabel("Username").fill("aiden");
    await page.getByLabel("Password").fill("test-password");
    await Promise.all([page.waitForURL(baseURL + "/"), page.getByRole("button", { name: "Sign in" }).click()]);
    await page.waitForFunction(() => window.htmx);

    const progress = page.locator("form.progress-form");
    const panel = page.locator("#progress-panel");
    const saveProgress = () => settle(page, () => progress.getByRole("button", { name: "Save progress" }).click());

    await page.locator("#total_hours").fill("4");
    await page.locator("#night_hours").fill("1");
    await saveProgress();
    assert.deepEqual(await statuses(panel), ["Progress saved"]);
    assert.deepEqual(await alerts(progress), []);

    // A rejected save shows the server's message and drops the stale success.
    await page.locator("#night_hours").fill("4.5");
    await saveProgress();
    assert.deepEqual(await alerts(progress), ["Night hours cannot be more than total hours"]);
    assert.deepEqual(await statuses(panel), []);
    assert.equal(await page.locator("#night_hours").inputValue(), "4.5");
    assert.equal(await panel.getByLabel("1.0 night hours").count(), 1, "rejected hours were shown as saved");

    await page.locator("#night_hours").fill("1");
    await page.locator("#permit_issue_date").fill("2099-01-01");
    await saveProgress();
    assert.deepEqual(await alerts(progress), ["Permit issue date cannot be in the future"]);
    assert.equal(await page.locator("#permit_issue_date").inputValue(), "2099-01-01");

    await page.locator("#permit_issue_date").fill("2025-01-01");
    await saveProgress();
    assert.deepEqual(await alerts(progress), []);
    assert.deepEqual(await statuses(panel), ["Progress saved"]);

    // The row locators re-resolve after htmx swaps a saved row.
    const row = page.locator("#requirement-use-of-lane");
    const other = page.locator("#requirement-quick-stop");
    const update = (form, title) => settle(page, () => form.getByRole("button", { name: "Update " + title }).click());
    const clear = (form, title) => settle(page, () => form.getByRole("button", { name: "Clear rating for " + title }).click());

    await row.locator("input[value=good]").check();
    await row.locator("[name=rated_on]").fill("2025-01-01");
    await row.locator("[name=notes]").fill("Original");
    await update(row, "Use of lane");
    assert.deepEqual(await statuses(row), ["Saved"]);
    assert.deepEqual(await alerts(row), []);

    await row.locator("[name=rated_on]").fill("2099-01-01");
    await row.locator("[name=notes]").fill("Retry later");
    await update(row, "Use of lane");
    assert.deepEqual(await alerts(row), ["Last rated date cannot be in the future"]);
    assert.deepEqual(await statuses(row), []);
    assert.equal(await row.locator("[name=rated_on]").inputValue(), "2099-01-01");
    assert.equal(await row.locator("[name=notes]").inputValue(), "Retry later");
    assert.equal(await row.locator("input[value=good]").isChecked(), true);
    assert.deepEqual(await alerts(other), [], "another row showed this row's error");
    assert.deepEqual(await alerts(progress), [], "the progress form showed a row's error");

    // Errors from the nested clear button land in its row and replace the last one.
    await row.locator("[name=notes]").fill("x".repeat(1001));
    await clear(row, "Use of lane");
    assert.deepEqual(await alerts(row), ["Notes must be 1000 characters or fewer"]);

    await row.locator("[name=rated_on]").fill("2025-02-01");
    await row.locator("[name=notes]").fill("Fixed");
    await update(row, "Use of lane");
    assert.deepEqual(await alerts(row), []);
    assert.deepEqual(await statuses(row), ["Saved"]);
    assert.equal(await row.locator("[name=notes]").inputValue(), "Fixed");

    // A successful clear also removes an earlier error.
    await row.locator("[name=rated_on]").fill("2099-01-01");
    await update(row, "Use of lane");
    assert.deepEqual(await alerts(row), ["Last rated date cannot be in the future"]);
    await clear(row, "Use of lane");
    assert.deepEqual(await alerts(row), []);
    assert.deepEqual(await statuses(row), ["Saved"]);
    assert.equal(await row.locator("input[name=rating]:checked").count(), 0);

    // Error text is shown as text, never parsed as markup.
    const markup = '<img src=x onerror="window.injected=true">Bad <b>input</b>';
    await page.route("**/requirements/quick-stop", (route) =>
      route.fulfill({ status: 400, contentType: "text/plain; charset=utf-8", body: markup + "\n" }),
    );
    await update(other, "Quick stop");
    assert.deepEqual(await alerts(other), [markup]);
    assert.equal(await other.locator("[role=alert] *").count(), 0);
    assert.equal(await page.evaluate(() => window.injected), undefined);
    await page.unrouteAll();

    // Only 400 bodies are shown; other failures keep their existing behavior.
    await page.route("**/requirements/quick-stop", (route) =>
      route.fulfill({ status: 500, contentType: "text/plain; charset=utf-8", body: "Unable to save requirement\n" }),
    );
    await update(other, "Quick stop");
    assert.deepEqual(await alerts(other), []);
    await page.unrouteAll();

    // A row sends one save at a time, so an update and a clear never race on
    // the server. While a save is held in flight, the row's controls are
    // disabled and trying the other one sends nothing; other rows stay usable.
    let held = [];
    await page.route("**/requirements/use-of-lane", (route) => {
      held.push(route);
    });
    const rowState = (form) =>
      form.evaluate((el) => ({
        rating: el.querySelector("input[name=rating]:checked")?.value ?? "",
        ratedOn: el.querySelector("[name=rated_on]").value,
        notes: el.querySelector("[name=notes]").value,
      }));
    const controls = (form) => form.locator("button, input");
    const updateRow = () => row.getByRole("button", { name: "Update Use of lane" }).click();
    const clearRow = () => row.getByRole("button", { name: "Clear rating for Use of lane" }).click();
    // Every way to send another save from the row: clicks that bypass the
    // disabled controls, and scripted submits that bypass them entirely.
    const secondSaves = [
      () => row.getByRole("button", { name: "Update Use of lane" }).click({ force: true }),
      () => row.getByRole("button", { name: "Clear rating for Use of lane" }).click({ force: true }),
      () => row.evaluate((form) => form.requestSubmit()),
      () => row.evaluate((form) => window.htmx.trigger(form.querySelector(".clear-rating"), "click")),
      () => row.evaluate((form) => window.htmx.trigger(form, "submit")),
    ];
    // holdFirst sends a save with send and holds it before it reaches the
    // server. While it waits, the row is busy, other rows are not, and no
    // second save is sent. The returned release lets the first save through
    // and checks that no second save was queued behind it either.
    const holdFirst = async (send) => {
      await send();
      await until(() => held.length === 1);
      for (const control of await controls(row).all()) {
        assert.equal(await control.isDisabled(), true, "a row control stayed enabled during a save");
      }
      for (const control of await controls(other).all()) {
        assert.equal(await control.isDisabled(), false, "another row was disabled by this row's save");
      }
      const sent = await page.evaluate(() => window.sent);
      for (const save of secondSaves) {
        await save();
      }
      assert.equal(await page.evaluate(() => window.sent), sent, "a second save was sent while the first was in flight");
      assert.equal(held.length, 1);
      return async () => {
        await settle(page, () => held.shift().continue());
        assert.equal(await page.evaluate(() => window.sent), sent, "a second save was queued behind the first");
        assert.equal(held.length, 0);
      };
    };
    // through sends a request with click and lets it reach the server.
    const through = async (click) => {
      await click();
      await until(() => held.length === 1);
      await settle(page, () => held.shift().continue());
    };
    // reloadedRow reloads the dashboard and returns the row's saved state.
    const reloadedRow = async () => {
      await page.reload();
      await page.waitForFunction(() => window.htmx);
      return rowState(row);
    };

    // The review's case: an update to "bad", then a clear. The clear can only
    // be sent after the update has been saved, so the clear is what persists.
    await row.locator("input[value=bad]").check();
    await row.locator("[name=rated_on]").fill("2025-03-01");
    await row.locator("[name=notes]").fill("Older");
    await (await holdFirst(updateRow))();
    assert.deepEqual(await statuses(row), ["Saved"]);
    assert.deepEqual(await rowState(row), { rating: "bad", ratedOn: "2025-03-01", notes: "Older" });
    await through(clearRow);
    assert.deepEqual(await statuses(row), ["Saved"]);
    const cleared = await rowState(row);
    assert.deepEqual(cleared, { rating: "", ratedOn: "", notes: "Older" });
    assert.deepEqual(await reloadedRow(), cleared, "the saved row differs from what the page showed");
    assert.ok(!(await page.locator("#practice-focus").innerText()).includes("Use of lane"));

    // A held update the server rejects: the row stays busy until the
    // rejection arrives, then shows it with the rejected values and is usable
    // again. A corrected save then persists.
    const tooLong = "<b>" + "x".repeat(1000);
    await row.locator("input[value=good]").check();
    await row.locator("[name=rated_on]").fill("2025-04-01");
    await row.locator("[name=notes]").fill(tooLong);
    await (await holdFirst(updateRow))();
    assert.deepEqual(await alerts(row), ["Notes must be 1000 characters or fewer"]);
    assert.equal(await row.locator("[role=alert] *").count(), 0);
    assert.deepEqual(await statuses(row), []);
    assert.deepEqual(await rowState(row), { rating: "good", ratedOn: "2025-04-01", notes: tooLong }, "the rejected values were not kept");
    for (const control of await controls(row).all()) {
      assert.equal(await control.isDisabled(), false, "a rejected row stayed disabled");
    }
    assert.deepEqual(await reloadedRow(), cleared, "a rejected update changed the saved row");

    await row.locator("input[value=good]").check();
    await row.locator("[name=rated_on]").fill("2025-04-01");
    await row.locator("[name=notes]").fill("Corrected");
    await through(updateRow);
    assert.deepEqual(await alerts(row), []);
    assert.deepEqual(await statuses(row), ["Saved"]);
    const corrected = { rating: "good", ratedOn: "2025-04-01", notes: "Corrected" };
    assert.deepEqual(await rowState(row), corrected);
    assert.deepEqual(await reloadedRow(), corrected, "the corrected save did not persist");

    // The reverse direction: a clear, then an update. The update can only be
    // sent after the clear has been saved, so the update is what persists.
    await (await holdFirst(clearRow))();
    assert.deepEqual(await statuses(row), ["Saved"]);
    assert.deepEqual(await rowState(row), { rating: "", ratedOn: "", notes: "Corrected" });
    await row.locator("input[value=fair]").check();
    await row.locator("[name=rated_on]").fill("2025-05-01");
    await row.locator("[name=notes]").fill("After clear");
    await through(updateRow);
    assert.deepEqual(await statuses(row), ["Saved"]);
    const updated = { rating: "fair", ratedOn: "2025-05-01", notes: "After clear" };
    assert.deepEqual(await rowState(row), updated);
    assert.deepEqual(await reloadedRow(), updated, "the update after a clear did not persist");
    await page.unrouteAll();

    // The progress form also sends one save at a time. While a rejected save
    // is held, its controls are disabled and even a scripted resubmit is
    // neither sent nor queued. The rejection then shows with the entered
    // values, and a correction saves without leaving the old error behind.
    held = [];
    await page.route("**/profile", (route) => {
      held.push(route);
    });
    const progressForm = page.locator("#progress-form");
    const hours = async () => [await page.locator("#total_hours").inputValue(), await page.locator("#night_hours").inputValue()];
    await page.locator("#total_hours").fill("6");
    await page.locator("#night_hours").fill("6.5");
    await progressForm.getByRole("button", { name: "Save progress" }).click();
    await until(() => held.length === 1);
    for (const control of await controls(progressForm).all()) {
      assert.equal(await control.isDisabled(), true, "a progress control stayed enabled during a save");
    }
    for (const control of await controls(other).all()) {
      assert.equal(await control.isDisabled(), false, "a row was disabled by the progress save");
    }
    const sent = await page.evaluate(() => window.sent);
    await progressForm.getByRole("button", { name: "Save progress" }).click({ force: true });
    await progressForm.evaluate((form) => form.requestSubmit());
    await progressForm.evaluate((form) => window.htmx.trigger(form, "submit"));
    await settle(page, () => held.shift().continue());
    assert.equal(await page.evaluate(() => window.sent), sent, "a second progress save was sent or queued during the first");
    assert.equal(held.length, 0);
    assert.deepEqual(await alerts(progressForm), ["Night hours cannot be more than total hours"]);
    assert.deepEqual(await statuses(panel), []);
    assert.deepEqual(await hours(), ["6", "6.5"], "the rejected hours were not kept");
    for (const control of await controls(progressForm).all()) {
      assert.equal(await control.isDisabled(), false, "a rejected progress form stayed disabled");
    }
    await page.reload();
    await page.waitForFunction(() => window.htmx);
    assert.deepEqual(await hours(), ["4.0", "1.0"], "a rejected progress save changed the saved hours");

    await page.locator("#total_hours").fill("6");
    await page.locator("#night_hours").fill("2");
    await through(() => progressForm.getByRole("button", { name: "Save progress" }).click());
    assert.deepEqual(await alerts(progressForm), []);
    assert.deepEqual(await statuses(panel), ["Progress saved"]);
    await page.reload();
    await page.waitForFunction(() => window.htmx);
    assert.deepEqual(await hours(), ["6.0", "2.0"], "the corrected progress did not persist");
    await page.unrouteAll();

    assert.deepEqual(problems, []);
  } finally {
    await browser.close();
  }
})().catch((err) => {
  console.error(err);
  process.exit(1);
});
