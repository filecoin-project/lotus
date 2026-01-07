#!/usr/bin/env bash
set -euo pipefail

# Runs each prompt file via Codex CLI multiple times (idempotent execution).
# Usage:
#   RUNS=2 ./prompts2/run_prompts.sh
# Environment:
#   RUNS  - number of passes for each prompt (default: 2)

RUNS="${RUNS:-2}"

if ! command -v codex >/dev/null 2>&1; then
  echo "Error: codex CLI not found in PATH. Install or export PATH accordingly." >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Collect prompt files (exclude README and non-numbered files), sorted lexicographically.
shopt -s nullglob
PROMPTS=( "${SCRIPT_DIR}"/[0-9][0-9]-*.md )

if [[ ${#PROMPTS[@]} -eq 0 ]]; then
  echo "No prompt files found in ${SCRIPT_DIR}. Expected files like 00-*.md" >&2
  exit 1
fi

echo "Found ${#PROMPTS[@]} prompt(s). Running each ${RUNS} time(s)." >&2
echo "Repo root: ${REPO_ROOT}" >&2

# Environment preamble for Codex CLI: set cwd, approvals, and sandbox mode.
ENV_PREAMBLE() {
  cat <<EOF
<environment_context>
  <cwd>${REPO_ROOT}</cwd>
  <approval_policy>never</approval_policy>
  <sandbox_mode>danger-full-access</sandbox_mode>
  <network_access>enabled</network_access>
  <shell>bash</shell>
</environment_context>

EOF
}

for prompt_file in "${PROMPTS[@]}"; do
  echo "===> Prompt: ${prompt_file}" >&2
  for ((i=1; i<=RUNS; i++)); do
    echo "--- Run ${i}/${RUNS}: ${prompt_file}" >&2
    PAYLOAD="$(ENV_PREAMBLE; cat "${prompt_file}")"
    (
      cd "${REPO_ROOT}"
      codex exec --dangerously-bypass-approvals-and-sandbox "${PAYLOAD}"
    ) || {
      echo "WARN: codex exec failed for ${prompt_file} (run ${i})." >&2
      exit 1
    }
  done
done

echo "All prompts executed." >&2

