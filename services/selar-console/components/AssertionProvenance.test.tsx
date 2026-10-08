import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it } from "vitest";
import { AssertionProvenance } from "./AssertionProvenance";

afterEach(() => { document.body.innerHTML = ""; });

async function render(link: Parameters<typeof AssertionProvenance>[0]["link"]) {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  const container = document.createElement("div"); document.body.append(container);
  const root = createRoot(container);
  await act(async () => { root.render(<AssertionProvenance link={link} />); });
  return { container, root };
}

it("says a comparison paper's claim about another work is not that work's own paper", async () => {
  const { container, root } = await render({
    assertion_scope: "reported_about_other", asserting_document_title: "RAC paper (fabricated)",
    source_document_id: "doc-rac", source_quote: "we re-implement a modified SagaLLM", review_revision: 2,
  });
  expect(container.textContent).toContain("Asserted by RAC paper (fabricated)");
  expect(container.textContent).toContain("not that work's own paper");
  expect(container.textContent).toContain("we re-implement a modified SagaLLM");
  expect(container.querySelector("a")?.getAttribute("href")).toBe("/reader?docId=doc-rac");
  await act(async () => root.unmount());
});

it("labels own-work assertions without claiming more", async () => {
  const { container, root } = await render({ assertion_scope: "own_work", asserting_document_title: "RAC paper" });
  expect(container.textContent).toContain("the document's own work");
  expect(container.textContent).not.toContain("another work");
  await act(async () => root.unmount());
});
