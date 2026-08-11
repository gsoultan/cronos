package yaml

// APIVersion is the only version this build reads.
//
// Checked rather than ignored. A file written for a later version may parse
// cleanly here and mean something else — a field that gained a default, a
// value that changed shape — and the failure would be wrong output rather than
// a refusal.
const APIVersion = "cronos.dev/v1"
