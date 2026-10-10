/* eslint-disable @typescript-eslint/no-require-imports */
// Local full-stack annotation regression. Same invented seed as reader-browser-check.cjs.
// SEED, BASE, CHROME, OUT; NODE_PATH may point at an external Playwright install.
const { chromium } = require("playwright");
const fs = require("fs");
const assert = require("node:assert/strict");
const seed = JSON.parse(fs.readFileSync(process.env.SEED, "utf8").trim().split("\n").pop());
const base = process.env.BASE || "http://localhost:18852";
const out = process.env.OUT || "annotation-shots";
fs.mkdirSync(out, { recursive: true });
const results = [];
function check(name, value) { assert.ok(value, name); results.push({ name, ok: true }); console.log(`PASS ${name}`); }
(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.CHROME });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, permissions: ["clipboard-read", "clipboard-write"] });
    const page = await context.newPage();
    const errors = []; page.on("pageerror", e => errors.push(e.message));
    const login = await context.request.post(`${base}/api/auth/login`, { data: { email: seed.email, password: seed.password } }); assert.equal(login.status(),200);
    const doc = seed.docs.at(-1);
    const list = async () => { const r = await context.request.get(`${base}/api/documents/${doc}/annotations`); assert.equal(r.status(),200); return r.json(); };
    const before = (await list()).map(a => a.id);
    // The list stays open across jumps on wide screens; only open it when closed.
    async function openList() { const b=page.getByRole("button",{name:"My highlights",exact:true}); if(await b.getAttribute("aria-expanded")!=="true") await b.click(); await page.locator('aside[aria-label="My highlights"]').waitFor(); }
    async function open(pageNumber=2) {
      await page.goto(`${base}/reader?docId=${doc}&page=${pageNumber}`);
      const skip=page.getByRole("button",{name:"Continue reading",exact:true});
      await Promise.race([skip.waitFor(),page.locator('.rd-page .textLayer').first().waitFor()]);
      if(await skip.isVisible()) await skip.click();
      await page.locator(`.rd-page[data-page="${pageNumber}"] .textLayer span`).first().waitFor();
      await page.waitForTimeout(500);
    }
    async function select() {
      const span=page.locator('.rd-page[data-page="2"] .textLayer span').filter({hasText:"Paragraph 1"}).first();
      await span.scrollIntoViewIfNeeded();
      return span.evaluate(el => {
        const r=document.createRange();r.selectNodeContents(el);const s=window.getSelection();s.removeAllRanges();s.addRange(r);
        el.dispatchEvent(new PointerEvent("pointerup",{bubbles:true}));return r.toString();
      });
    }
    await open();
    const quote=await select();
    const menu=page.getByRole("toolbar",{name:"Selection actions"});await menu.waitFor();
    await menu.getByRole("button",{name:"Copy",exact:true}).click();
    check("selection Copy preserves exact source text",await page.evaluate(()=>navigator.clipboard.readText())===quote);
    await menu.getByRole("button",{name:"Note",exact:true}).click();
    const editor=page.getByRole("dialog",{name:"Add note",exact:true});await editor.waitFor();
    check("note editor receives keyboard focus",await editor.getByRole("textbox",{name:"Note",exact:true}).evaluate(el=>el===document.activeElement));
    check("note editor overlays the page instead of pushing it down",await page.evaluate(()=>{const s=document.querySelector(".rd-scroll").getBoundingClientRect(),t=document.querySelector(".rd-toolbar").getBoundingClientRect();return Math.abs(s.top-t.bottom)<2;}));
    await editor.getByRole("radio",{name:"Pink",exact:true}).click();
    await editor.getByRole("textbox",{name:"Note",exact:true}).fill("My own reflection, not a verified relationship.");
    await editor.getByRole("textbox",{name:"Note",exact:true}).press("ControlOrMeta+Enter");await editor.waitFor({state:"hidden"});
    let a=(await list()).find(a=>!before.includes(a.id));
    check("note, selected color and exact anchor persist",a && a.comment==="My own reflection, not a verified relationship." && a.color==="coral" && a.anchor.exact===quote && a.anchor.end-a.anchor.start===quote.length && !!a.anchor.source_hash);
    const id=a.id;
    await openList();
    let row=page.locator(`[data-annotation-id="${id}"]`);await row.waitFor();
    await row.getByRole("button",{name:"Edit",exact:true}).click();
    const edit=page.getByRole("dialog",{name:"Edit note",exact:true});
    await edit.getByRole("textbox",{name:"Note",exact:true}).fill("Changed then cancelled");await edit.getByRole("textbox",{name:"Note",exact:true}).press("Escape");
    check("Escape cancels without mutating saved note",(await list()).find(a=>a.id===id).comment===a.comment);
    await row.getByRole("button",{name:"Edit",exact:true}).click();
    await edit.getByRole("textbox",{name:"Note",exact:true}).fill("Edited reflection");
    await edit.getByRole("radio",{name:"Green",exact:true}).click();
    await page.route(`**/api/annotations/${id}`, async route => {
      if (route.request().method()==="PATCH") { await route.fulfill({status:503,contentType:"application/json",body:JSON.stringify({error:"Synthetic retry check"})}); await page.unroute(`**/api/annotations/${id}`); }
      else await route.continue();
    });
    await edit.getByRole("button",{name:"Save",exact:true}).click();await edit.getByRole("alert").waitFor();
    check("failed edit stays open and leaves persisted note untouched",await edit.isVisible() && (await list()).find(item=>item.id===id).comment===a.comment);
    await edit.getByRole("button",{name:"Save",exact:true}).click();await edit.waitFor({state:"hidden"});
    a=(await list()).find(a=>a.id===id);check("edit persists note and color without rewriting quote",a.comment==="Edited reflection" && a.color==="sage" && a.anchor.exact===quote);
    // Clicking the saved mark on the page opens it for editing (Zotero/Preview behaviour).
    await page.keyboard.press("Escape");
        const own=page.locator(`.rd-user[data-mark-id="${id}"]`).first();await own.scrollIntoViewIfNeeded();
    const ob=await own.boundingBox();await page.mouse.click(ob.x+ob.width/2,ob.y+ob.height/2);
    const reopened=page.getByRole("dialog",{name:"Edit note",exact:true});await reopened.waitFor();
    check("clicking a saved mark opens its note",await reopened.getByRole("textbox",{name:"Note",exact:true}).inputValue()==="Edited reflection");
    await reopened.getByRole("button",{name:"Cancel",exact:true}).click();await reopened.waitFor({state:"hidden"});
    // One click on a colour swatch saves a highlight in that colour.
    const before2=(await list()).length;
    const other=page.locator('.rd-page[data-page="2"] .textLayer span').filter({hasText:"Paragraph 3"}).first();await other.scrollIntoViewIfNeeded();
    await other.evaluate(el=>{const r=document.createRange();r.selectNodeContents(el);const s=getSelection();s.removeAllRanges();s.addRange(r);el.dispatchEvent(new PointerEvent("pointerup",{bubbles:true}));});
    await menu.waitFor();await menu.getByRole("radio",{name:"Orange",exact:true}).click();await menu.waitFor({state:"hidden"});
    const swatched=(await list()).find(x=>!before.includes(x.id)&&x.id!==id);
    check("one-click swatch saves a highlight in that colour",(await list()).length===before2+1&&swatched?.color==="wheat");
    await context.request.delete(`${base}/api/annotations/${swatched.id}`);
    await open(9);await openList();row=page.locator(`[data-annotation-id="${id}"]`);
    check("highlight list survives reload",await row.innerText().then(s=>s.includes("Edited reflection")&&s.includes(quote)));
    await row.getByRole("button",{name:"Copy quote",exact:true}).click();check("list copies stored exact quote",await page.evaluate(()=>navigator.clipboard.readText())===quote);
    await row.getByRole("button",{name:"Jump to page 2",exact:true}).click();await page.waitForTimeout(800);
    check("list jump scrolls to anchored passage",(await page.locator('.page-indicator').first().textContent()).startsWith("2 /") && await page.locator('.rd-flash').count()>0);
    const geometry=()=>page.locator('.rd-page[data-page="2"] .rd-user.c-sage').first().evaluate(el=>({left:el.style.left,top:el.style.top,width:el.style.width,height:el.style.height}));
    const beforeZoom=await geometry();await page.getByRole("combobox",{name:"Zoom",exact:true}).selectOption("1.5");await page.waitForTimeout(800);const afterZoom=await geometry();
    check("zoom reanchors marks to the same text",Math.abs(parseFloat(beforeZoom.left)-parseFloat(afterZoom.left))<1 && Math.abs(parseFloat(beforeZoom.top)-parseFloat(afterZoom.top))<1);
    await page.setViewportSize({width:700,height:900});
    // Existing mobile side panels intentionally overlay the document until closed.
    await page.evaluate(() => document.querySelector('.reader-scrim')?.click());
    await page.getByRole("combobox",{name:"Zoom",exact:true}).selectOption("fit-width");await page.waitForTimeout(800);
    check("reflow retains source-bound highlight",await page.locator('.rd-user.c-sage').count()>0);
    await page.screenshot({path:`${out}/highlight-reflow.png`});
    await openList();row=page.locator(`[data-annotation-id="${id}"]`);
    await row.getByRole("button",{name:"Delete",exact:true}).click();check("delete asks for confirmation",await row.getByRole("button",{name:"Confirm delete",exact:true}).isVisible());
    await row.getByRole("button",{name:"Keep highlight",exact:true}).click();check("cancel deletion keeps persisted highlight",(await list()).some(a=>a.id===id));
    await row.getByRole("button",{name:"Delete",exact:true}).click();await row.getByRole("button",{name:"Confirm delete",exact:true}).click();await row.waitFor({state:"hidden"});
    check("confirmed deletion persists",!(await list()).some(a=>a.id===id));
    await open();await openList();check("deleted mark stays absent after reload",await page.locator(`[data-annotation-id="${id}"]`).count()===0);
    // A failed PDF load explains itself and recovers with "Try again".
    let failPdf=true;
    // Drop the on-device PDF cache so the reopen really goes to the network.
    await page.evaluate(async()=>{if(self.caches)for(const k of await caches.keys())await caches.delete(k);});
    await page.route("**/api/documents/*/pdf",route=>failPdf?route.fulfill({status:503,body:"synthetic outage"}):route.continue());
    await page.goto(`${base}/reader?docId=${doc}&page=1`);
    const retry=page.getByRole("button",{name:"Try again",exact:true});
    const gate=page.getByRole("button",{name:"Continue reading",exact:true});
    await Promise.race([gate.waitFor({timeout:60000}),retry.waitFor({timeout:60000})]);
    if(await gate.isVisible())await gate.click();
    await retry.waitFor({timeout:60000});
    check("PDF load failure shows a retry",await page.getByText("Couldn't open this PDF").isVisible());
    failPdf=false;await retry.click();await page.locator('.rd-page .textLayer span').first().waitFor({timeout:60000});
    check("Try again recovers the document",await page.locator('.rd-page').count()>0);
    await page.unroute("**/api/documents/*/pdf");
    check("no browser runtime errors",errors.length===0);
  } finally {fs.writeFileSync(`${out}/results.json`,JSON.stringify(results,null,2));await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1});
