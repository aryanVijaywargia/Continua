#!/usr/bin/env python3
"""Generate Python types from OpenAPI spec."""

import subprocess
import sys
from pathlib import Path

def main():
    repo_root = Path(__file__).parent.parent.parent.parent
    openapi_path = repo_root / "contracts" / "openapi" / "openapi.bundle.yaml"
    output_path = Path(__file__).parent.parent / "src" / "continua" / "types.py"

    if not openapi_path.exists():
        print(f"❌ OpenAPI bundle not found: {openapi_path}")
        sys.exit(1)

    cmd = [
        "datamodel-codegen",
        "--input", str(openapi_path),
        "--output", str(output_path),
        "--input-file-type", "openapi",
        "--output-model-type", "pydantic_v2.BaseModel",
        "--use-schema-description",
        "--field-constraints",
        "--use-double-quotes",
        "--target-python-version", "3.10",
    ]

    result = subprocess.run(cmd, capture_output=True, text=True)

    if result.returncode != 0:
        print(f"❌ Generation failed:\n{result.stderr}")
        sys.exit(1)

    print(f"✅ Generated {output_path}")

if __name__ == "__main__":
    main()
