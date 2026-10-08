"""Packaging regression: the worker image must contain every local module main imports."""

import ast
import re
from pathlib import Path

HERE = Path(__file__).parent


def _local_imports(module: str, seen: set[str]) -> set[str]:
    if module in seen or not (HERE / f"{module}.py").exists():
        return seen
    seen.add(module)
    tree = ast.parse((HERE / f"{module}.py").read_text())
    for node in ast.walk(tree):
        names = []
        if isinstance(node, ast.Import):
            names = [alias.name.split(".")[0] for alias in node.names]
        elif isinstance(node, ast.ImportFrom) and node.module and node.level == 0:
            names = [node.module.split(".")[0]]
        for name in names:
            _local_imports(name, seen)
    return seen


def test_dockerfile_copies_every_local_module_main_needs():
    dockerfile = (HERE / "Dockerfile").read_text()
    copied: set[str] = set()
    for line in dockerfile.splitlines():
        match = re.match(r"\s*COPY\s+(.+?)\s+\S+\s*$", line)
        if not match:
            continue
        for source in match.group(1).split():
            if source == "*.py":
                copied |= {path.stem for path in HERE.glob("*.py")}
            elif source.endswith(".py"):
                copied.add(Path(source).stem)
    needed = _local_imports("main", set())
    assert needed <= copied, f"Dockerfile is missing modules: {sorted(needed - copied)}"


def test_modal_image_includes_every_local_module_main_needs():
    import modal_app

    needed = _local_imports("main", set()) | _local_imports("worker_trigger", set())
    assert needed <= set(modal_app.LOCAL_MODULES), sorted(needed - set(modal_app.LOCAL_MODULES))
