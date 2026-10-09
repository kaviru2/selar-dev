import { readFileSync } from "node:fs";
import path from "node:path";
import { expect, it } from "vitest";

const workflow = readFileSync(path.resolve(__dirname, "../../../.github/workflows/pages.yml"), "utf8");

it("keeps the optional docs site manual-only, without provisioning Pages or hiding errors", () => {
  const triggers = workflow.match(/^on:\n([\s\S]*?)(?=^\S)/m)?.[1].trim();
  expect(triggers).toBe("workflow_dispatch:");
  expect(workflow).toContain("uses: actions/configure-pages@v5");
  expect(workflow).toContain("uses: actions/deploy-pages@v4");
  expect(workflow).not.toMatch(/enablement:|continue-on-error:/);
});
