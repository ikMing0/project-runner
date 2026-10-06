package main

import "regexp"

// Severity detection uses visible text, while stored log output retains SGR
// codes so the frontend can render the original colours.
var logANSIControls = regexp.MustCompile(`(?:\x1b\[|\x{009b})[0-?]*[ -/]*[@-~]|(?:\x1b\]|\x{009d})[^\x07\x1b\x{009c}]*(?:\x07|\x1b\\|\x{009c})|\x1b[ -/]*[@-Z\\-_]|(?:\x1b\[|\x{009b})[0-?]*[ -/]*$|(?:\x1b\]|\x{009d})[^\x07\x1b\x{009c}]*$|[\x00-\x08\x0b-\x1f\x7f-\x{009f}]`)

func plainLogText(text string) string { return logANSIControls.ReplaceAllString(text, "") }
