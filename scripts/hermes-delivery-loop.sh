#!/usr/bin/env bash

set -euo pipefail

start_issue="${1:-7}"
end_issue="${2:-16}"
phase_turns="${HERMES_PHASE_MAX_TURNS:-100}"
compose_file="infra/local/docker-compose.hermes.yaml"
log_dir="${TMPDIR:-/tmp}/hermes-delivery-$(date +%Y%m%d-%H%M%S)"

if [[ -x /opt/hermes/bin/hermes ]]; then
    hermes_prefix=(/opt/hermes/bin/hermes)
else
    hermes_prefix=(docker compose -f "$compose_file" exec -T hermes /opt/hermes/bin/hermes)
fi

if [[ ! "$start_issue" =~ ^[0-9]+$ || ! "$end_issue" =~ ^[0-9]+$ || "$start_issue" -gt "$end_issue" ]]; then
    echo "usage: $0 [start_issue] [end_issue]" >&2
    exit 2
fi

if [[ -n "$(git status --porcelain)" ]]; then
    echo "refusing to start with a dirty worktree; preserve or commit existing changes first" >&2
    git status --short >&2
    exit 1
fi

mkdir -p "$log_dir"
echo "Hermes compact delivery logs: $log_dir"

run_phase() {
    local name="$1"
    local prompt="$2"
    local log="$log_dir/$name.log"

    echo "==> $name"
    "${hermes_prefix[@]}" chat --yolo --oneshot --quiet \
        --max-turns "$phase_turns" -q "$prompt" 2>&1 | tee "$log"
}

for issue in $(seq "$start_issue" "$end_issue"); do
    branch="feat/issue-${issue}-autonomous-delivery"

    git switch main
    git pull --ff-only origin main
    if git show-ref --verify --quiet "refs/heads/$branch"; then
        echo "local branch already exists: $branch" >&2
        exit 1
    fi
    git switch -c "$branch"

    run_phase "issue-${issue}-developer" "Work only on GitHub issue #${issue} in the current /workspace branch. Act as hermes-dev. Read the required project documents and relevant skill, run graphify query before source exploration, implement production code only, and do not commit, push, or delegate. Keep the change scoped to issue #${issue}. Report changed files, checks, and blockers concisely."
    run_phase "issue-${issue}-tester" "Work only on GitHub issue #${issue} in the current /workspace branch. Act as hermes-tester. Read TESTING.md and the relevant skill, run graphify query before source exploration, inspect the Developer changes, add or correct tests only, run required tests, and do not modify production behavior, commit, push, or delegate. Report changed test files, commands, results, and blockers concisely."

    review_log="$log_dir/issue-${issue}-review-0.log"
    run_phase "issue-${issue}-review-0" "Review only GitHub issue #${issue} in the current /workspace branch. Act as hermes-reviewer. Read the required project documents and relevant skill, run graphify query before source exploration, inspect the complete diff, do not edit files, and classify every finding as production_fix, test_fix, both, or human_blocker. End with APPROVED only when no blocker or high-severity finding remains."

    approved=0
    for cycle in 1 2 3; do
        if grep -Eiq '(^|[^[:alpha:]])APPROVED([^[:alpha:]]|$)' "$review_log"; then
            approved=1
            break
        fi
        if grep -Eiq 'human_blocker|human blocker' "$review_log"; then
            echo "human blocker reported for issue #$issue; see $review_log" >&2
            exit 1
        fi

        findings="$(tail -c 16000 "$review_log")"
        run_phase "issue-${issue}-fix-${cycle}-developer" "Fix only production findings for GitHub issue #${issue} in the current /workspace branch. Act as hermes-dev. Read the required documents, run graphify query before source exploration, and use this read-only reviewer report as scope. Do not commit, push, delegate, or change tests unless required by a production fix. If there are no production findings, make no changes. Reviewer report:\n${findings}"
        run_phase "issue-${issue}-fix-${cycle}-tester" "Fix only test findings for GitHub issue #${issue} in the current /workspace branch. Act as hermes-tester. Read TESTING.md, run graphify query before source exploration, and use this read-only reviewer report as scope. Do not change production behavior, commit, push, or delegate. If there are no test findings, make no changes. Reviewer report:\n${findings}"
        review_log="$log_dir/issue-${issue}-review-${cycle}.log"
        run_phase "issue-${issue}-review-${cycle}" "Review only GitHub issue #${issue} in the current /workspace branch. Act as hermes-reviewer. Read the required documents and relevant skill, run graphify query before source exploration, inspect the complete diff, do not edit files, and classify findings. End with APPROVED only when no blocker or high-severity finding remains."
    done

    if ! grep -Eiq '(^|[^[:alpha:]])APPROVED([^[:alpha:]]|$)' "$review_log"; then
        echo "review did not approve issue #$issue after three fix cycles; see $review_log" >&2
        exit 1
    fi

    run_phase "issue-${issue}-delivery" "Deliver the approved implementation for GitHub issue #${issue} on the current branch. Act as hermes-delivery and read /workspace/.agents/skills/hermes-delivery/SKILL.md. This is a fresh, non-delegating delivery session: do not invoke delegate_task or any Developer, Tester, or Reviewer agent. Inspect the complete diff and run required checks, then commit the complete change, push the branch, create a linked pull request targeting main with Closes #${issue}, merge it using GitHub MCP, verify the PR is merged and closed, mark the corresponding GitHub Project item Done, switch the local checkout to main, and pull origin/main. Do not ask for approval and do not use gh. Stop and report the exact operation if any delivery step fails."

    if [[ -n "$(git status --porcelain)" ]]; then
        echo "delivery left a dirty worktree for issue #$issue" >&2
        git status --short >&2
        exit 1
    fi
    git switch main
    git pull --ff-only origin main
done

echo "Completed issues $start_issue through $end_issue"
