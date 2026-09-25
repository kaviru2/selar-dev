"""Test-only entrypoint: real worker routes/retrieval, deterministic Gemini boundary.

Run only against the isolated synthetic CI database. Never use in production.
"""
import os
import re
from types import SimpleNamespace

if os.environ.get("SELAR_SYNTHETIC_E2E") != "1" or "selar_e2e" not in os.environ.get("TEST_DATABASE_URL", ""):
    raise RuntimeError("synthetic chat worker requires the isolated pgvector CI database")

import main

main.DATABASE_URL = os.environ["TEST_DATABASE_URL"]
main.embed_query_text = lambda _question: [1.0] + [0.0] * 3071


class SyntheticModels:
    def generate_content(self, **kwargs):
        prompt = kwargs["contents"]
        # Only the ordinary comparison question reaches generation. Do not
        # fabricate a citation: use the label of the retrieved comparison chunk.
        match = re.search(r"\[S(\d+)\] Birch comparison paper,", prompt)
        if not match or "modified CedarAgent baseline on BeaconBench" not in prompt:
            raise AssertionError("comparison source missing from retrieved evidence")
        return SimpleNamespace(text=f"Birch reports a modified CedarAgent baseline on BeaconBench [S{match.group(1)}].")


main.client = SimpleNamespace(models=SyntheticModels())
app = main.app
