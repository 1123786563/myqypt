---
status: accepted
---

# Require versioned migration and rollback contracts

Every Product Version declares its Migration Class, backup and tested-restore requirements, rollback support, compatible source-version range, and expected downtime. Cell upgrades pass compatibility testing, backup, isolated Restore Rehearsal, drain, deployment, migration, smoke testing, and an observation window before promotion; forward-only or destructive failures recover from verified backup rather than mislabeling an old-image deployment as rollback.
