---
description: Build a prioritized plan from the GitHub Project board and open issues
subtask: false
---

Pull the current state of the MaestroVault GitHub Project board and open issues, then build a prioritized plan.

Here is the project board state:
!`gh project item-list 2 --owner rmkohlman --format json --limit 200`

Here are the open issues:
!`gh issue list --repo rmkohlman/MaestroVault --state open --limit 50 --json number,title,labels,body,assignees,milestone,createdAt --jq '.[] | "### #\(.number): \(.title)\nLabels: \(.labels | map(.name) | join(", "))\nCreated: \(.createdAt)\n\(.body)\n---"'`

$ARGUMENTS

Based on the above, create a structured plan:
1. Identify any **In Progress** items that need to be resumed first
2. Group **Todo** items by priority (high/medium/low) considering labels, age, and dependencies
3. For each item, note the assigned Agent and Effort estimate
4. Flag any items that are missing Agent, Effort, or Sprint fields — these need enrichment
5. Flag any items that depend on or block other items
6. Suggest an implementation order respecting the pipeline: Design → Tests → Implement → Verify → Ship
7. Use the TodoWrite tool to create actionable tasks from this plan
