package memory

/*
#cgo CFLAGS: -I${SRCDIR} -I${SRCDIR}/../../vendor/github.com/mattn/go-sqlite3 -DSQLITE_CORE
#cgo linux LDFLAGS: -lm
#include "sqlite3.h"
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

	_ "github.com/mattn/go-sqlite3"
)

// Force reference to ensure C symbols from go-sqlite3 are linked
// var _ *sqlite3.SQLiteConn

// AutoRegisterVec automatically registers the sqlite-vec extension on all
// future go-sqlite3 connections created in this process.
func AutoRegisterVec() {
	fmt.Println("[assistclaw] Auto-registering sqlite-vec extension...")
	rc := C.assistclaw_sqlite3_vec_auto_with_rc()
	if rc != 0 {
		panic(fmt.Sprintf("failed to auto-register sqlite-vec: rc=%d", rc))
	}
	fmt.Println("[assistclaw] sqlite-vec auto-registration successful")
}
