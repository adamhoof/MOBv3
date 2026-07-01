//go:build !admin

package main

func handleOptionalCommand(_ ctlConfig, _ string, _ []string) bool {
	return false
}

func optionalUsage() string {
	return ""
}

func optionalCommands() string {
	return ""
}
