import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

// Match scripts/with-env.sh: credentials are literal, including $ and #.
export function parseLiteralEnv(source: string): Record<string, string> {
  const values: Record<string, string> = {};
  for (const line of source.split(/\r?\n/)) {
    const match = /^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$/.exec(line);
    if (!match) continue;
    let value = match[2].trim();
    if (
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"))
    )
      value = value.slice(1, -1);
    values[match[1]] = value;
  }
  return values;
}

export function loadLiteralEnv(
  directory: string,
  mode: string,
): Record<string, string> {
  const values: Record<string, string> = {};
  for (const name of [
    ".env",
    ".env.local",
    `.env.${mode}`,
    `.env.${mode}.local`,
  ]) {
    const path = join(directory, name);
    if (existsSync(path))
      Object.assign(values, parseLiteralEnv(readFileSync(path, "utf8")));
  }
  return values;
}
