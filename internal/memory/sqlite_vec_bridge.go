package memory

/*
#cgo CFLAGS: -I. -I../../vendor/github.com/mattn/go-sqlite3 -DSQLITE_CORE
#cgo linux LDFLAGS: -lm
#include "sqlite3.h"
#include "sqlite-vec.h"

int assistclaw_sqlite3_vec_init(sqlite3 *db) {
    return sqlite3_vec_init(db, NULL, NULL);
}
*/
import "C"
import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/mattn/go-sqlite3"
)

// RegisterVec manually registers the sqlite-vec extension on a go-sqlite3 connection.
func RegisterVec(conn *sqlite3.SQLiteConn) error {
	// go-sqlite3 does not expose the raw *sqlite3 pointer in a clean way,
	// but we can use reflection to get the "db" field if we really need to.
	// HOWEVER, there is a better way: use the driver's ConnectHook.

	// We use reflection to get the internal *C.sqlite3 from go-sqlite3.SQLiteConn
	v := reflect.ValueOf(conn).Elem()
	f := v.FieldByName("db")
	if !f.IsValid() {
		return fmt.Errorf("could not find db field in SQLiteConn")
	}

	db := (*C.sqlite3)(unsafe.Pointer(f.Pointer()))
	rc := C.assistclaw_sqlite3_vec_init(db)
	if rc != C.SQLITE_OK {
		return fmt.Errorf("failed to register sqlite-vec: rc=%d", int(rc))
	}
	fmt.Println("[assistclaw] sqlite-vec registered on connection")
	return nil
}
