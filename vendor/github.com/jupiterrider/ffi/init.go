//go:build (freebsd || linux || windows || darwin) && (amd64 || arm64)

package ffi

import (
	"fmt"
	"runtime"
)

// filename is the name or path to the libffi shared library.
var filename string

// initErr stores runtime loader/proc resolution failures so callers can fail-open.
var initErr error

// Available reports whether libffi was loaded and all required symbols were resolved.
func Available() bool { return initErr == nil }

// InitError returns the initialization error when Available() is false.
func InitError() error { return initErr }

func init() {
	if len(filename) == 0 {
		switch runtime.GOOS {
		case "freebsd", "linux":
			filename = "libffi.so.8"
		case "windows":
			filename = "libffi-8.dll"
		case "darwin":
			filename = "libffi.8.dylib"
		}
	}

	libffi, err := Load(filename)
	if err != nil {
		initErr = fmt.Errorf("ffi: load %q: %w", filename, err)
		return
	}

	prepCif, err = libffi.Get("ffi_prep_cif")
	if err != nil {
		initErr = fmt.Errorf("ffi: resolve ffi_prep_cif: %w", err)
		return
	}

	prepCifVar, err = libffi.Get("ffi_prep_cif_var")
	if err != nil {
		initErr = fmt.Errorf("ffi: resolve ffi_prep_cif_var: %w", err)
		return
	}

	call, err = libffi.Get("ffi_call")
	if err != nil {
		initErr = fmt.Errorf("ffi: resolve ffi_call: %w", err)
		return
	}

	closureAlloc, err = libffi.Get("ffi_closure_alloc")
	if err != nil {
		initErr = fmt.Errorf("ffi: resolve ffi_closure_alloc: %w", err)
		return
	}

	closureFree, err = libffi.Get("ffi_closure_free")
	if err != nil {
		initErr = fmt.Errorf("ffi: resolve ffi_closure_free: %w", err)
		return
	}

	prepClosureLoc, err = libffi.Get("ffi_prep_closure_loc")
	if err != nil {
		initErr = fmt.Errorf("ffi: resolve ffi_prep_closure_loc: %w", err)
		return
	}

	getStructOffsets, err = libffi.Get("ffi_get_struct_offsets")
	if err != nil {
		initErr = fmt.Errorf("ffi: resolve ffi_get_struct_offsets: %w", err)
		return
	}

	// Because ffi_get_version and ffi_get_version_number just exist since libffi 3.5.0, we don't fail hard here.
	getVersion, _ = libffi.Get("ffi_get_version")
	getVersionNumber, _ = libffi.Get("ffi_get_version_number")
}
