#!/usr/bin/env python3
"""Generate Python types from OpenAPI spec."""

import subprocess
import sys
from pathlib import Path

import yaml


def apply_nullable_allof_fixes(openapi_path: Path, output_path: Path) -> None:
    """Patch datamodel-code-generator output for nullable allOf refs.

    The generator currently drops `nullable: true` when a property is modeled as
    `allOf: [$ref: ...]`, which is how the shared OpenAPI pipeline expresses
    required nullable object refs. We preserve those semantics here by reading
    the OpenAPI schema and rewriting the generated field annotations to `T | None`
    while keeping the fields required.
    """

    spec = yaml.safe_load(openapi_path.read_text())
    schemas = spec.get("components", {}).get("schemas", {})
    nullable_ref_fields: dict[str, dict[str, str]] = {}

    for schema_name, schema in schemas.items():
        properties = schema.get("properties", {})
        for field_name, field_schema in properties.items():
            if not field_schema.get("nullable"):
                continue

            refs = field_schema.get("allOf")
            if not isinstance(refs, list) or len(refs) != 1:
                continue

            ref = refs[0].get("$ref")
            if not isinstance(ref, str) or not ref.startswith("#/components/schemas/"):
                continue

            target_type = ref.rsplit("/", 1)[-1]
            nullable_ref_fields.setdefault(schema_name, {})[field_name] = target_type

    if not nullable_ref_fields:
        return

    lines = output_path.read_text().splitlines()
    current_class: str | None = None
    patched_lines: list[str] = []

    for line in lines:
        stripped = line.strip()
        if stripped.startswith("class ") and stripped.endswith("(BaseModel):"):
            current_class = stripped.split()[1].split("(")[0]
        elif stripped and not line.startswith(" "):
            current_class = None

        if current_class and current_class in nullable_ref_fields:
            for field_name, target_type in nullable_ref_fields[current_class].items():
                prefix = f"    {field_name}: {target_type}"
                if line.startswith(prefix) and "| None" not in line:
                    line = line.replace(prefix, f"{prefix} | None", 1)
                    break

        patched_lines.append(line)

    output_path.write_text("\n".join(patched_lines) + "\n")

def main():
    repo_root = Path(__file__).parent.parent.parent.parent
    openapi_path = repo_root / "contracts" / "openapi" / "openapi.bundle.yaml"
    output_path = Path(__file__).parent.parent / "src" / "continua" / "types.py"

    if not openapi_path.exists():
        print(f"❌ OpenAPI bundle not found: {openapi_path}")
        sys.exit(1)

    cmd = [
        sys.executable,
        "-m",
        "datamodel_code_generator",
        "--input", str(openapi_path),
        "--output", str(output_path),
        "--input-file-type", "openapi",
        "--disable-timestamp",
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

    apply_nullable_allof_fixes(openapi_path, output_path)

    print(f"✅ Generated {output_path}")

if __name__ == "__main__":
    main()
