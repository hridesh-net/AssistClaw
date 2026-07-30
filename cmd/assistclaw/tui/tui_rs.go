package tui

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo LDFLAGS: -L${SRCDIR}/../tui_rs/target/release -lassistclaw_tui -lpthread -ldl
#include "tui_rs.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// Init initializes the Rust TUI runtime. Must be called once before any TUI functions.
func Init() error {
	if C.tui_init() != 0 {
		return fmt.Errorf("tui_init failed")
	}
	return nil
}

// Shutdown cleans up the Rust TUI runtime.
func Shutdown() {
	C.tui_shutdown()
}

// ReplStart starts the REPL TUI. Non-blocking; spawns a Rust thread.
func ReplStart(configJSON string) error {
	cConfig := C.CString(configJSON)
	defer C.free(unsafe.Pointer(cConfig))

	if C.tui_repl_start(cConfig) != 0 {
		return fmt.Errorf("tui_repl_start failed")
	}
	return nil
}

// ReplStop signals the REPL TUI to shut down.
func ReplStop() {
	C.tui_repl_stop()
}

// ReplSendToken streams an agent token to the REPL chat viewport.
func ReplSendToken(token string) {
	cToken := C.CString(token)
	defer C.free(unsafe.Pointer(cToken))
	C.tui_repl_send_token(cToken)
}

// ReplSendTool shows a tool-call indicator in the REPL.
func ReplSendTool(name, inputJSON string) {
	cName := C.CString(name)
	cInput := C.CString(inputJSON)
	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cInput))
	C.tui_repl_send_tool(cName, cInput)
}

// ReplSendDone signals the agent finished successfully.
func ReplSendDone(iterations, totalTokens int) {
	C.tui_repl_send_done(C.int(iterations), C.int(totalTokens))
}

// ReplSendError signals an agent error.
func ReplSendError(message string) {
	cMsg := C.CString(message)
	defer C.free(unsafe.Pointer(cMsg))
	C.tui_repl_send_error(cMsg)
}

// StatusRun starts the status dashboard. Blocks until user quits.
func StatusRun(infoJSON string) error {
	cInfo := C.CString(infoJSON)
	defer C.free(unsafe.Pointer(cInfo))

	if C.tui_status_run(cInfo) != 0 {
		return fmt.Errorf("tui_status_run failed")
	}
	return nil
}

// StatusUpdate sends a live status update to the dashboard.
func StatusUpdate(statusJSON string) {
	cStatus := C.CString(statusJSON)
	defer C.free(unsafe.Pointer(cStatus))
	C.tui_status_update(cStatus)
}

// OnboardRun runs the onboarding wizard. Blocks until complete or cancelled.
// Returns the new config JSON, or empty string if cancelled.
func OnboardRun(configJSON string) (string, error) {
	cConfig := C.CString(configJSON)
	defer C.free(unsafe.Pointer(cConfig))

	result := C.tui_onboard_run(cConfig)
	if result == nil {
		return "", fmt.Errorf("onboard cancelled")
	}
	defer C.tui_free_string(result)

	return C.GoString(result), nil
}

// SkillsRun runs the skills configurator. Blocks until complete or cancelled.
// Returns the selected skills JSON, or empty string if cancelled.
func SkillsRun(catalogJSON, currentSkillsJSON string) (string, error) {
	cCatalog := C.CString(catalogJSON)
	cCurrent := C.CString(currentSkillsJSON)
	defer C.free(unsafe.Pointer(cCatalog))
	defer C.free(unsafe.Pointer(cCurrent))

	result := C.tui_skills_run(cCatalog, cCurrent)
	if result == nil {
		return "", fmt.Errorf("skills configure cancelled")
	}
	defer C.tui_free_string(result)

	return C.GoString(result), nil
}

// RenderBanner returns the full splash banner (robot + info pane + tagline)
// as an ANSI-coloured string ready to print.
func RenderBanner(version, sessionID string, providers, skills int) string {
	cVer := C.CString(version)
	cSess := C.CString(sessionID)
	defer C.free(unsafe.Pointer(cVer))
	defer C.free(unsafe.Pointer(cSess))
	out := C.tui_render_banner(cVer, cSess, C.int(providers), C.int(skills))
	if out == nil {
		return ""
	}
	defer C.tui_free_string(out)
	return C.GoString(out)
}

// RenderOnboardBanner returns the shorter onboarding banner.
func RenderOnboardBanner(version string) string {
	cVer := C.CString(version)
	defer C.free(unsafe.Pointer(cVer))
	out := C.tui_render_onboard_banner(cVer)
	if out == nil {
		return ""
	}
	defer C.tui_free_string(out)
	return C.GoString(out)
}

// RenderCLIHeader returns the compact stderr CLI header. termWidth=0 → default.
func RenderCLIHeader(version string, termWidth int) string {
	cVer := C.CString(version)
	defer C.free(unsafe.Pointer(cVer))
	out := C.tui_render_cli_header(cVer, C.int(termWidth))
	if out == nil {
		return ""
	}
	defer C.tui_free_string(out)
	return C.GoString(out)
}

// RenderVersionBlock returns the styled `version` command output.
func RenderVersionBlock(version string) string {
	cVer := C.CString(version)
	defer C.free(unsafe.Pointer(cVer))
	out := C.tui_render_version_block(cVer)
	if out == nil {
		return ""
	}
	defer C.tui_free_string(out)
	return C.GoString(out)
}

// PollEvent polls for the next TUI event from Rust.
// Returns empty string if no event is pending.
func PollEvent() string {
	event := C.tui_poll_event()
	if event == nil {
		return ""
	}
	defer C.tui_free_string(event)
	return C.GoString(event)
}
