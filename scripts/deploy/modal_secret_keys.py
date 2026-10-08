"""Print the NAMES (never values) of keys stored in a Modal secret.

Used by scripts/deploy/env-sync.sh:

    SELAR_MODAL_SECRET=selar-worker-secrets modal run scripts/deploy/modal_secret_keys.py

Modal's API returns no key names for a secret, so this runs a tiny ephemeral
function with the secret attached and prints the sorted environment variable
names that the secret injected (compared with a function without the secret).
"""

import os

import modal

SECRET_NAME = os.environ.get("SELAR_MODAL_SECRET", "selar-worker-secrets")

app = modal.App("selar-env-sync-probe")
image = modal.Image.debian_slim(python_version="3.11")


@app.function(image=image, timeout=60)
def baseline_names() -> list[str]:
    return sorted(os.environ)


@app.function(image=image, secrets=[modal.Secret.from_name(SECRET_NAME)], timeout=60)
def secret_names() -> list[str]:
    return sorted(os.environ)


@app.local_entrypoint()
def main() -> None:
    base = set(baseline_names.remote())
    names = [name for name in secret_names.remote() if name not in base]
    print("SELAR_SECRET_KEYS_BEGIN")
    for name in names:
        print(name)
    print("SELAR_SECRET_KEYS_END")
