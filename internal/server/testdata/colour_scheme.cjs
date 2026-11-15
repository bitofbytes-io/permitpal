// Loads the dashboard in Chromium with prefers-color-scheme light and dark and
// checks the colours its components actually render with. TestColourSchemeInBrowser
// runs it with the server URL as its argument and the Playwright module path in
// PERMITPAL_PLAYWRIGHT_MODULE.
const assert = require("node:assert/strict");
const { chromium } = require(process.env.PERMITPAL_PLAYWRIGHT_MODULE);

const baseURL = process.argv[2];

// Each probe names a component, the property it paints and the scheme token it
// must consume. The rendered colour is compared with the token resolved in the
// page, so a rule that stops using the token, or a later rule that overrides
// it, fails here whatever the stylesheet looks like.
const probes = [
  { selector: "body", property: "color", token: "--ink" },
  { selector: "html", property: "background-color", token: "--paper" },
  { selector: ".side-panel", property: "background-color", token: "--surface" },
  { selector: ".readiness-card", property: "background-color", token: "--surface" },
  { selector: ".entry-note", property: "color", token: "--note" },
  { selector: ".checklist-head", property: "color", token: "--head-ink" },
  { selector: ".checklist-head", property: "background-color", token: "--head-bg" },
  { selector: ".checklist-head span:nth-child(3)", property: "color", token: "--bad-label" },
  { selector: ".checklist-head span:nth-child(4)", property: "color", token: "--fair-label" },
  { selector: ".checklist-head span:nth-child(5)", property: "color", token: "--good-label" },
  { selector: ".skill-row--bad", property: "background-color", token: "--row-bad" },
  { selector: ".skill-row--fair", property: "background-color", token: "--row-fair" },
  { selector: ".skill-row--good", property: "background-color", token: "--row-good" },
  { selector: ".skill-description", property: "color", token: "--note-2" },
  { selector: ".checklist-toolbar button:not(.is-selected)", property: "color", token: "--button-ink" },
  { selector: ".checklist-toolbar button:not(.is-selected)", property: "background-color", token: "--button-bg" },
  { selector: ".checklist-reference a", property: "color", token: "--link" },
  { selector: "input[type=date]", property: "background-color", token: "--input-bg" },
  { selector: "input[type=date]", property: "color", token: "--ink" },
];

// colours returns each probe's rendered colour and its token's resolved colour.
// A probe element asks the browser to resolve var(--token) to the same rgb()
// form as the computed style.
function colours(probes) {
  const probe = document.createElement("span");
  document.body.append(probe);
  const out = probes.map(({ selector, property, token }) => {
    const el = document.querySelector(selector);
    if (!el) {
      return { selector, property, token, missing: true };
    }
    probe.style.cssText = `${property}: var(${token})`;
    return {
      selector,
      property,
      token,
      rendered: getComputedStyle(el)[property],
      want: getComputedStyle(probe)[property],
    };
  });
  probe.remove();
  return out;
}

// luminance is the relative luminance of an rgb()/rgba() colour string.
function luminance(colour) {
  const [r, g, b] = colour
    .match(/[\d.]+/g)
    .slice(0, 3)
    .map((v) => {
      const c = Number(v) / 255;
      return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
    });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

async function visit(browser, colorScheme) {
  const context = await browser.newContext({ colorScheme, viewport: { width: 1280, height: 900 } });
  try {
    const page = await context.newPage();
    await page.goto(baseURL + "/login");
    await page.getByLabel("Username").fill("aiden");
    await page.getByLabel("Password").fill("test-password");
    await Promise.all([page.waitForURL(baseURL + "/"), page.getByRole("button", { name: "Sign in" }).click()]);
    await page.waitForFunction(() => window.htmx);
    // Rate three skills bad, fair and good so each rating background is rendered.
    // The rows may be hidden by the checklist view, so submit them directly.
    for (const rating of ["bad", "fair", "good"]) {
      if (await page.$(`.skill-row--${rating}`)) {
        continue; // The store keeps ratings from the earlier visit.
      }
      await page.evaluate((rating) => {
        const row = document.querySelector("form.skill-row:not(.skill-row--bad):not(.skill-row--fair):not(.skill-row--good)");
        row.querySelector(`input[name=rating][value=${rating}]`).checked = true;
        row.requestSubmit();
      }, rating);
      await page.waitForSelector(`.skill-row--${rating}`, { state: "attached" });
    }
    return {
      results: await page.evaluate(colours, probes),
      bodyBackground: await page.evaluate(() => getComputedStyle(document.body).backgroundImage),
      pageTop: await page.evaluate(() => {
        const probe = document.createElement("span");
        probe.style.cssText = "background-color: var(--page-top)";
        document.body.append(probe);
        const colour = getComputedStyle(probe).backgroundColor;
        probe.remove();
        return colour;
      }),
      colorScheme: await page.evaluate(() => getComputedStyle(document.documentElement).colorScheme),
    };
  } finally {
    await context.close();
  }
}

(async () => {
  const browser = await chromium.launch();
  try {
    const light = await visit(browser, "light");
    const dark = await visit(browser, "dark");

    for (const [scheme, page] of [["light", light], ["dark", dark]]) {
      assert.equal(page.colorScheme, "light dark", `${scheme}: native controls must follow the scheme`);
      for (const r of page.results) {
        const name = `${scheme}: ${r.selector} ${r.property} (${r.token})`;
        assert.ok(!r.missing, `${name}: no element matches`);
        assert.equal(r.rendered, r.want, `${name} does not render the token's colour`);
      }
    }

    // The dark scheme must really be dark and differ from the light one in
    // every probed component.
    light.results.forEach((l, i) => {
      const d = dark.results[i];
      assert.notEqual(d.rendered, l.rendered, `${d.selector} ${d.property} is the same in both schemes`);
    });
    const darkBackground = dark.results.find((r) => r.selector === "html").rendered;
    const lightBackground = light.results.find((r) => r.selector === "html").rendered;
    assert.ok(luminance(darkBackground) < 0.05, `dark page background ${darkBackground} is not dark`);
    assert.ok(luminance(lightBackground) > 0.8, `light page background ${lightBackground} is not light`);
    const darkText = dark.results.find((r) => r.selector === "body").rendered;
    assert.ok(luminance(darkText) > 0.5, `dark body text ${darkText} is not light`);
    assert.match(dark.bodyBackground, /linear-gradient/, "body keeps its gradient background");
    assert.notEqual(dark.pageTop, light.pageTop, "--page-top is the same in both schemes");
  } finally {
    await browser.close();
  }
})().catch((err) => {
  console.error(err);
  process.exit(1);
});
