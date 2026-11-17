package macos

// Failure is a check of a release that failed: its name and what it found.
type Failure struct{ Name, Detail string }
