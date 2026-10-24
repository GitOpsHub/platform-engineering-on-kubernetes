"""Parse every YAML file given on the command line and check each document.

Multi-document files are supported. Empty documents are skipped. Every document must be a
mapping with apiVersion and kind. Run it through uv so PyYAML is not installed globally:

    uv run --with pyyaml python hack/lint_yaml.py $(find . -name '*.yaml')
"""

import sys

import yaml


def main(paths):
    failures = 0
    for path in paths:
        with open(path) as f:
            for doc in yaml.safe_load_all(f):
                if doc is None:
                    continue
                if not isinstance(doc, dict) or "apiVersion" not in doc or "kind" not in doc:
                    print(f"{path}: document missing apiVersion or kind")
                    failures += 1
    print(f"lint: {len(paths)} files checked, {failures} problem(s)")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
