---
description: Build a prioritized plan from GitHub issues and feature requests
subtask: false
---

Look at the open GitHub issues and feature requests for this repo and build a prioritized todo list of work to do.

Here are the open issues:
!`gh issue list --state open --limit 50 --json number,title,labels,body,assignees,milestone,createdAt --jq '.[] | "### #\(.number): \(.title)\nLabels: \(.labels | map(.name) | join(", "))\nCreated: \(.createdAt)\n\(.body)\n---"'`

Here are the open feature requests (issues labeled "enhancement"):
!`gh issue list --state open --label enhancement --limit 30 --json number,title,body,createdAt --jq '.[] | "#\(.number): \(.title) - \(.body[0:200])"'`

$ARGUMENTS

Based on the above, create a structured plan:
1. Group items by priority (high/medium/low) considering labels, age, and dependencies
2. For each item, provide a brief summary of what needs to be done
3. Flag any items that depend on or block other items
4. Suggest an implementation order
5. Use the TodoWrite tool to create actionable tasks from this plan
