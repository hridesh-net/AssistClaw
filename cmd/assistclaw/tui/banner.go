package tui

import "fmt"

// PrintBanner prints the splash banner (rendered in Rust) to stdout.
func PrintBanner(ver, sessionID string, providers, skillsCount int) {
	fmt.Println(RenderBanner(ver, sessionID, providers, skillsCount))
}

// PrintOnboardBanner prints the shorter onboarding banner (rendered in Rust).
func PrintOnboardBanner(ver string) {
	fmt.Println(RenderOnboardBanner(ver))
}
