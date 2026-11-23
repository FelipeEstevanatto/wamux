// Package groups is reserved for the WhatsApp Cloud Groups API program.
//
// Groups are not part of pkg/cloud core. They are OBA-only, capped by Meta and
// have a different message-type matrix; enable the separate program with
// CLOUD_GROUPS only after its plan in docs/archive/plans/cloud-groups.md is
// implemented. Importing this package must never change core send behavior.
package groups
