// Scripted browser check for the reader (issue #91).
// Needs a running console + API seeded with an invented multi-page PDF and suggestions.
/* eslint-disable @typescript-eslint/no-require-imports */
// Run: npm i playwright@1.56 (outside the app), then
//   SEED=seed.json CHROME=/path/to/chrome BASE=http://localhost:3000 OUT=shots node scripts/reader-browser-check.cjs
// SEED is a JSON line: {"email","password","docs":[...ids]} for a throwaway account. Exits 1 on any failed check.
// Scripted browser check for the rebuilt reader (continuous scroll).
// Env: SEED (seed.out), BASE, CHROME, OUT. Exits non-zero on failed checks.
const { chromium } = require("playwright");
const fs = require("fs");
const seed = JSON.parse(fs.readFileSync(process.env.SEED, "utf8").trim().split("\n").pop());
const BASE = process.env.BASE || "http://localhost:18732";
const OUT = process.env.OUT || "shots/after";
fs.mkdirSync(OUT, { recursive: true });
const doc = seed.docs[seed.docs.length - 1];
const results = [];
function check(name, ok, detail) {
  results.push({ name, ok: !!ok, detail });
  console.log(`${ok ? "PASS" : "FAIL"} ${name}${detail !== undefined ? " " + JSON.stringify(detail) : ""}`);
}

(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.CHROME });
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  const errors = [];
  page.on("pageerror", (e) => errors.push(String(e)));
  page.on("console", (m) => { if (m.type() === "error" && !/favicon|Failed to load resource|Failed to fetch/.test(m.text())) errors.push(m.text()); });
  await page.goto(`${BASE}/login`);
  await ctx.request.post(`${BASE}/api/auth/login`, { data: { email: seed.email, password: seed.password } });
  await page.addInitScript(() => { if (!sessionStorage.getItem("cleared")) { localStorage.clear(); sessionStorage.setItem("cleared", "1"); } });

  const indicator = async () => (await page.locator(".page-indicator").first().textContent()).trim();
  const sc = page.locator(".rd-scroll");

  await page.goto(`${BASE}/reader?docId=${doc}&page=1`);
  await page.locator(".rd-page .textLayer").first().waitFor({ timeout: 60000 });
  await page.waitForTimeout(1200);
  await page.screenshot({ path: `${OUT}/01-initial.png` });
  const total = Number((await indicator()).split("/")[1]);
  check("indicator shows page 1 of N", (await indicator()).startsWith("1 /") && total >= 10, await indicator());
  const mounted = await page.locator(".rd-page .react-pdf__Page").count();
  check("virtualised: only nearby pages mounted", mounted > 0 && mounted <= 6, { mounted, total });
  check("all pages laid out (placeholders)", (await page.locator(".rd-page").count()) === total);

  // Continuous scroll advances the indicator.
  await page.mouse.move(800, 500);
  for (let i = 0; i < 6; i++) { await page.mouse.wheel(0, 600); await page.waitForTimeout(120); }
  await page.waitForTimeout(500);
  const afterWheel = Number((await indicator()).split("/")[0]);
  check("wheel scroll advances page indicator", afterWheel >= 2, await indicator());

  // Next page lands on the TOP of the next page.
  await page.getByRole("button", { name: "Next page" }).click();
  await page.waitForTimeout(500);
  const p = Number((await indicator()).split("/")[0]);
  const topDelta = await page.evaluate((n) => {
    const el = document.querySelector(`.rd-page[data-page="${n}"]`);
    const sc = document.querySelector(".rd-scroll");
    return Math.round(el.getBoundingClientRect().top - sc.getBoundingClientRect().top);
  }, p);
  check("Next page lands at top of target page (small margin)", p === afterWheel + 1 && topDelta >= 0 && topDelta <= 24, { p, topDelta });
  await page.screenshot({ path: `${OUT}/02-next-page-top.png` });

  // Keyboard
  await sc.focus();
  await page.keyboard.press("End");
  await page.waitForTimeout(400);
  check("End goes to last page", (await indicator()).startsWith(`${total} /`), await indicator());
  await page.keyboard.press("Home");
  await page.waitForTimeout(400);
  check("Home goes to page 1", (await indicator()).startsWith("1 /"), await indicator());
  await page.keyboard.press("PageDown");
  await page.waitForTimeout(300);
  await page.keyboard.press("ArrowRight");
  await page.waitForTimeout(300);
  check("PgDn + → advance one page each", (await indicator()).startsWith("3 /"), await indicator());

  // Page input
  const input = page.getByRole("textbox", { name: /Page number/ });
  await input.click();
  await input.fill("9");
  await input.press("Enter");
  await page.waitForTimeout(500);
  check("typing a page number jumps", (await indicator()).startsWith("9 /"), await indicator());

  // Zoom keeps position
  await sc.evaluate((el) => { el.scrollTop += 300; });
  await page.waitForTimeout(300);
  const anchorBefore = await page.evaluate(() => {
    const sc = document.querySelector(".rd-scroll");
    for (const el of document.querySelectorAll(".rd-page")) {
      const r = el.getBoundingClientRect(); const t = sc.getBoundingClientRect().top;
      if (r.bottom > t) return { page: el.dataset.page, frac: (t - r.top) / r.height };
    }
  });
  await page.getByRole("button", { name: "Zoom in" }).click();
  await page.getByRole("button", { name: "Zoom in" }).click();
  await page.waitForTimeout(900);
  const anchorAfter = await page.evaluate(() => {
    const sc = document.querySelector(".rd-scroll");
    for (const el of document.querySelectorAll(".rd-page")) {
      const r = el.getBoundingClientRect(); const t = sc.getBoundingClientRect().top;
      if (r.bottom > t) return { page: el.dataset.page, frac: (t - r.top) / r.height };
    }
  });
  check("zoom preserves reading position", anchorBefore.page === anchorAfter.page && Math.abs(anchorBefore.frac - anchorAfter.frac) < 0.03, { anchorBefore, anchorAfter });
  const crisp = await page.locator(".rd-page canvas").first().evaluate((c) => c.width >= c.clientWidth);
  check("zoomed canvas is re-rendered at resolution (not CSS-scaled)", crisp);
  await page.screenshot({ path: `${OUT}/03-zoomed.png` });
  await page.getByRole("combobox", { name: "Zoom" }).selectOption("fit-width");
  await page.waitForTimeout(600);

  // Thumbnails
  await page.getByRole("button", { name: "Page thumbnails" }).click();
  await page.waitForTimeout(800);
  await page.getByRole("button", { name: "Go to page 5" }).click();
  await page.waitForTimeout(600);
  check("thumbnail click jumps", (await indicator()).startsWith("5 /"), await indicator());
  await page.screenshot({ path: `${OUT}/04-thumbnails.png` });
  await page.getByRole("button", { name: "Page thumbnails" }).click();

  // Find
  await sc.focus();
  await page.keyboard.press(process.platform === "darwin" ? "Meta+f" : "Control+f");
  const find = page.getByRole("searchbox").or(page.locator(".rd-findbar input"));
  await find.first().fill("retrieval");
  await page.waitForTimeout(1500);
  const count = await page.locator(".rd-find-count").textContent();
  check("Ctrl/⌘+F finds matches across pages", /of \d+/.test(count), count);
  check("find highlights in text layer", (await page.locator("mark.rd-find").count()) > 0);
  await page.screenshot({ path: `${OUT}/05-find.png` });
  await find.first().press("Escape");

  // Selection menu: text selectable, menu appears, highlight persists.
  await page.goto(`${BASE}/reader?docId=${doc}&page=2`);
  await page.locator('.rd-page[data-page="2"] .textLayer span').first().waitFor({ timeout: 30000 });
  await page.waitForTimeout(800);
  check("deep link ?page=2 opens page 2", (await indicator()).startsWith("2 /"), await indicator());
  const span = page.locator('.rd-page[data-page="2"] .textLayer span').filter({ hasText: /\w{4,}/ }).nth(2);
  await span.scrollIntoViewIfNeeded();
  const box = await span.boundingBox();
  await page.mouse.move(box.x + 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width - 2, box.y + box.height / 2, { steps: 8 });
  await page.mouse.up();
  await page.waitForTimeout(400);
  const sel = await page.evaluate(() => String(window.getSelection()));
  check("text selection works", sel.trim().length > 3, sel.slice(0, 40));
  const menu = page.getByRole("toolbar", { name: "Selection actions" });
  check("selection toolbar appears", await menu.isVisible());
  await page.screenshot({ path: `${OUT}/06-selection.png` });
  const marksBefore = await page.locator(".rd-user").count();
  await menu.getByRole("button", { name: /Highlight/ }).first().click();
  await page.waitForTimeout(900);
  check("highlight created", (await page.locator(".rd-user").count()) > marksBefore);
  await page.screenshot({ path: `${OUT}/07-highlighted.png` });

  // Position memory: scroll to page 7, reload without ?page.
  await page.getByRole("textbox", { name: /Page number/ }).fill("7");
  await page.getByRole("textbox", { name: /Page number/ }).press("Enter");
  await page.waitForTimeout(1200);
  await page.goto(`${BASE}/reader?docId=${doc}`);
  await page.locator(".rd-page .textLayer").first().waitFor({ timeout: 30000 });
  await page.waitForTimeout(1000);
  check("last position remembered per document", (await indicator()).startsWith("7 /"), await indicator());
  check("URL tracks current page", page.url().includes("page=7"), page.url());

  // Suggestions: next match scrolls + flashes passage, popover opens without blocking selection.
  await page.getByRole("button", { name: "Go to next suggested passage" }).click();
  await page.waitForTimeout(1500);
  check("next suggestion flashes passage", (await page.locator(".rd-flash").count()) > 0, await indicator());
  await page.screenshot({ path: `${OUT}/08-next-suggestion-flash.png` });
  const ev = page.locator(".rd-evidence").first();
  if (await ev.count()) {
    const pe = await ev.evaluate((e) => getComputedStyle(e).pointerEvents);
    check("evidence marks don't block text selection", pe === "none", pe);
    const eb = await ev.boundingBox();
    await page.mouse.click(eb.x + eb.width / 2, eb.y + eb.height / 2);
    await page.waitForTimeout(400);
    check("clicking evidence opens popover", await page.getByRole("dialog", { name: "Suggested connection" }).isVisible());
    await page.screenshot({ path: `${OUT}/09-popover.png` });
    await page.keyboard.press("Escape");
    await page.waitForTimeout(200);
    check("Escape closes popover", (await page.getByRole("dialog", { name: "Suggested connection" }).count()) === 0);
  } else check("evidence marks rendered", false);

  // Sidebar "Show on page" scrolls in place (no app reload) and flashes.
  await page.getByRole("tab", { name: "About this reading" }).click();
  const details = page.locator("details.cx-passages > summary");
  if (await details.count()) {
    await details.click();
    await page.locator(".passage-card .reveal").nth(2).click();
    await page.waitForTimeout(1500);
    check("Show on page flashes target", (await page.locator(".rd-flash").count()) > 0, await indicator());
    await page.screenshot({ path: `${OUT}/10-show-on-page.png` });
  }

  // Mobile
  await page.setViewportSize({ width: 390, height: 844 });
  await page.waitForTimeout(1000);
  await page.screenshot({ path: `${OUT}/11-mobile.png` });
  const pageW = await page.locator(".rd-page").first().evaluate((e) => e.getBoundingClientRect().width);
  check("mobile: page fits width", pageW > 300 && pageW <= 390, pageW);
  // Panels persisted open from the desktop run float over the page here; close them via the scrim.
  await page.evaluate(() => (document.querySelector(".reader-scrim"))?.click());
  await page.waitForTimeout(300);
  const toggle = page.locator('.rd-toolbar button[aria-controls="reader-connections"]');
  check("mobile: Connections toggle visible", await toggle.isVisible());
  await toggle.click();
  await page.waitForTimeout(500);
  await page.screenshot({ path: `${OUT}/12-mobile-drawer.png` });
  check("mobile: Connections opens as an overlay", await page.locator("#reader-connections").isVisible());

  check("no page errors", errors.length === 0, errors.slice(0, 5));
  fs.writeFileSync(`${OUT}/results.json`, JSON.stringify(results, null, 2));
  await browser.close();
  const failed = results.filter((r) => !r.ok).length;
  console.log(`${results.length - failed}/${results.length} checks passed`);
  process.exit(failed ? 1 : 0);
})().catch((e) => { console.error(e); process.exit(2); });
