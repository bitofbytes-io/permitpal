// Drives the dashboard's htmx forms in Chromium and checks the save feedback
// each form shows. TestDashboardSaveFeedbackInBrowser runs it with the server
// URL as its argument and the Playwright module path in
// PERMITPAL_PLAYWRIGHT_MODULE.
const assert = require("node:assert/strict");
const { chromium } = require(process.env.PERMITPAL_PLAYWRIGHT_MODULE);

const baseURL = process.argv[2];

// countLoads counts finished requests in window.loaded, a task after every
// load listener the page added before sending, so htmx and the page have
// handled the response by the time it changes.
function countLoads() {
  window.loaded = 0;
  const send = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.send = function (...args) {
    this.addEventListener("loadend", () => setTimeout(() => window.loaded++));
    return send.apply(this, args);
  };
}

// settle runs action and waits until count more requests have been handled.
async function settle(page, action, count = 1) {
  const loaded = await page.evaluate(() => window.loaded);
  await action();
  await page.waitForFunction((want) => window.loaded >= want, loaded + count, { timeout: 5000 });
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
    await page.addInitScript(countLoads);
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

    // The update and the nested clear button send separate requests, so
    // their responses can finish in either order. Only a row's latest
    // request reports, even after an older one has swapped the row.
    let held = [];
    await page.route("**/requirements/use-of-lane", (route) => {
      held.push(route);
    });
    await row.locator("input[value=good]").check();
    await row.locator("[name=rated_on]").fill("2099-01-01");
    await row.getByRole("button", { name: "Update Use of lane" }).click();
    await row.getByRole("button", { name: "Clear rating for Use of lane" }).click();
    await until(() => held.length === 2);
    await settle(page, () => held[1].continue());
    await settle(page, () => held[0].continue());
    assert.deepEqual(await alerts(row), [], "a stale rejection was reported after a newer save");
    assert.deepEqual(await statuses(row), ["Saved"]);

    held = [];
    await row.locator("input[value=fair]").check();
    await row.locator("[name=rated_on]").fill("2025-03-01");
    await row.locator("[name=notes]").fill("Older");
    await row.getByRole("button", { name: "Update Use of lane" }).click();
    await row.locator("[name=notes]").fill("x".repeat(1001));
    await row.getByRole("button", { name: "Clear rating for Use of lane" }).click();
    await until(() => held.length === 2);
    await settle(page, () => held[0].continue());
    await settle(page, () => held[1].continue());
    assert.deepEqual(await alerts(row), ["Notes must be 1000 characters or fewer"], "the latest rejection was lost after an older save");
    assert.deepEqual(await statuses(row), []);
    await page.unrouteAll();

    assert.deepEqual(problems, []);
  } finally {
    await browser.close();
  }
})().catch((err) => {
  console.error(err);
  process.exit(1);
});
