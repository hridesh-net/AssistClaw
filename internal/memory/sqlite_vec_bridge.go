//go:build cgo

package memory

/*
#cgo CFLAGS: -I${SRCDIR} -I${SRCDIR}/../../vendor/github.com/mattn/go-sqlite3 -DSQLITE_CORE
#cgo linux LDFLAGS: -lm
#include "../../vendor/github.com/mattn/go-sqlite3/sqlite3-binding.h"
#include "sqlite-vec.h"

int assistclaw_sqlite3_vec_init(sqlite3 *db, char **pzErrMsg, const sqlite3_api_routines *pApi) {
    return sqlite3_vec_init(db, pzErrMsg, pApi);
}

int assistclaw_sqlite3_vec_auto_with_rc() {
    return sqlite3_auto_extension((void (*)(void))assistclaw_sqlite3_vec_init);
}
*/
import "C"

import (
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

// AutoRegisterVec automatically registers the sqlite-vec extension on all
// future go-sqlite3 connections created in this process.
func AutoRegisterVec() {
	fmt.Fprintf(os.Stderr, "[assistclaw] Auto-registering sqlite-vec extension...\n")
	rc := C.assistclaw_sqlite3_vec_auto_with_rc()
	if rc != 0 {
		fmt.Fprintf(os.Stderr, "[assistclaw] CRITICAL: failed to auto-register sqlite-vec: rc=%d\n", rc)
		// We don't panic here to let the app try to start, but it will likely fail later.
	} else {
		fmt.Fprintf(os.Stderr, "[assistclaw] sqlite-vec auto-registration successful\n")
	}
}
