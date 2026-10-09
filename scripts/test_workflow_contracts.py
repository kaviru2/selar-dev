"""Actions cost-control contracts. Run: python scripts/test_workflow_contracts.py.

Requires PyYAML; these tests validate configuration, not service behavior.
"""
from pathlib import Path
import unittest

import yaml

WORKFLOWS = Path(__file__).resolve().parents[1] / ".github" / "workflows"
PR_WORKFLOWS = ("ci", "coverage", "synthetic-e2e", "claude-review", "dependency-review")
GROUP = "${{ github.workflow }}-${{ github.event_name }}-${{ github.event.pull_request.number || github.run_id }}"
CANCEL = "${{ github.event_name == 'pull_request' }}"


def load(name):
    return yaml.safe_load((WORKFLOWS / f"{name}.yml").read_text())


class WorkflowContracts(unittest.TestCase):
    def test_cancellation_is_scoped_to_workflow_and_pr(self):
        for name in PR_WORKFLOWS:
            with self.subTest(workflow=name):
                concurrency = load(name).get("concurrency", {})
                self.assertEqual(concurrency.get("group"), GROUP)
                self.assertEqual(concurrency.get("cancel-in-progress"), CANCEL)
        # PR updates collide; different PRs/workflows and all non-PR runs do not.
        def group(workflow, event, pr, run):
            return f"{workflow}-{event}-{pr or run}"
        self.assertEqual(group("CI", "pull_request", 1, 10), group("CI", "pull_request", 1, 11))
        self.assertNotEqual(group("CI", "pull_request", 1, 10), group("CI", "pull_request", 2, 11))
        self.assertNotEqual(group("CI", "pull_request", 1, 10), group("Coverage", "pull_request", 1, 10))
        for event in ("push", "merge_group", "workflow_dispatch", "issue_comment"):
            self.assertNotEqual(group("CI", event, None, 10), group("CI", event, None, 11))


    def test_timeouts_and_dependency_caches_keep_installs(self):
        for name in ("ci", "coverage", "synthetic-e2e"):
            for job_id, job in load(name)["jobs"].items():
                with self.subTest(workflow=name, job=job_id):
                    self.assertEqual(job["runs-on"], "ubuntu-latest")
                    self.assertGreaterEqual(job.get("timeout-minutes", 0), 15)
                    self.assertLessEqual(job.get("timeout-minutes", 999), 25)
                    steps = job["steps"]
                    uses = [step.get("uses", "") for step in steps]
                    for step in steps:
                        if step.get("uses", "").startswith("actions/setup-python@"):
                            self.assertEqual(step["with"].get("cache"), "pip")
                            self.assertEqual(step["with"].get("cache-dependency-path"), "services/selar-worker/requirements.txt")
                        if step.get("uses", "").startswith("actions/setup-node@"):
                            self.assertEqual(step["with"].get("cache"), "pnpm")
                            self.assertEqual(step["with"].get("cache-dependency-path"), "services/selar-console/pnpm-lock.yaml")
                            self.assertLess(uses.index("pnpm/action-setup@v4"), uses.index(step["uses"]))
                    commands = "\n".join(step.get("run", "") for step in steps)
                    if "actions/setup-node@v4" in uses:
                        self.assertIn("pnpm install --frozen-lockfile", commands)
                    if "actions/setup-python@v5" in uses:
                        self.assertIn("pip install -r", commands)


    def test_optional_reviews_preserve_manual_route_and_check_names(self):
        claude = load("claude-review")["jobs"]["review"]
        automatic, manual = claude["if"].strip().split(" ||\n")
        self.assertIn("vars.CLAUDE_REVIEW_ENABLED == 'true'", automatic)
        self.assertEqual(manual, "(github.event_name == 'issue_comment' &&\n github.event.issue.pull_request != null &&\n contains(github.event.comment.body, '@claude'))")
        self.assertEqual(claude["name"], "Claude / PR Review")
        self.assertTrue(any(step.get("id") == "review-config" for step in claude["steps"]))
        self.assertTrue(any(step.get("uses") == "anthropics/claude-code-action@v1" for step in claude["steps"]))
        dependency = load("dependency-review")["jobs"]["dependency-review"]
        self.assertEqual(dependency.get("if"), "${{ vars.DEPENDENCY_REVIEW_ENABLED == 'true' }}")
        self.assertEqual(dependency["name"], "Dependency Review")
        review = next(step for step in dependency["steps"] if step.get("uses") == "actions/dependency-review-action@v4")
        self.assertEqual(review["with"]["fail-on-severity"], "high")
        self.assertEqual(review["with"]["deny-licenses"], "GPL-3.0, AGPL-3.0")
        self.assertEqual(review["if"], "steps.dependency-graph.outputs.enabled == 'true'")


    def test_existing_check_names_and_merge_group_are_preserved(self):
        expected = {
            "ci": {"build-api": "Build Go API", "build-console": "Build Next.js Console", "check-worker": "Check Python Worker"},
            "coverage": {"go-coverage": "Go API Coverage", "console-coverage": "Console Coverage", "worker-coverage": "Worker Coverage"},
            "synthetic-e2e": {"service-boundaries": None},
        }
        for name, jobs in expected.items():
            workflow = load(name)
            self.assertEqual({key: job.get("name") for key, job in workflow["jobs"].items()}, jobs)
            triggers = workflow.get("on", workflow.get(True))
            self.assertIn("pull_request", triggers)
            self.assertNotIn("paths", triggers["pull_request"])
            for job in workflow["jobs"].values():
                self.assertNotIn("if", job)
        self.assertIn("merge_group", load("ci").get("on", load("ci").get(True)))
        self.assertEqual(load("db-backup")["concurrency"]["cancel-in-progress"], False)
        self.assertEqual(load("pages")["jobs"]["deploy"]["environment"]["name"], "github-pages")


if __name__ == "__main__":
    unittest.main(verbosity=2)
