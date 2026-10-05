// Package benchmarks holds end-to-end performance scenarios for the paths that
// actually matter in production: the group-send crypto pipeline, the HTTP
// request pipeline for the hot endpoints, webhook signing, message persistence
// and the per-process idle-memory footprint.
//
// # Philosophy
//
// Measure before optimizing. Every benchmark here is paired with a Test that
// runs the same code path once and asserts its output, so a benchmark never
// measures a path that is silently broken. Benchmarks that need a real database
// skip unless EVO_BENCH_POSTGRES_DSN is set, matching the convention already
// used by pkg/message/repository.
//
// See README.md for how to run each scenario and how to read the results.
package benchmarks
