package tui

import (
	"reflect"
	"testing"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/dbtest"
)

// TestSwitchDatabase_RepointsEveryService is a STRUCTURAL test, not a behavioral
// one, and it exists because of a real bug that reached a user.
//
// switchDatabase re-points each service at the newly opened file by hand, one
// assignment per field. Adding a service field to App and updating only NewApp
// leaves that field bound to the STARTUP database: the app then reads and writes
// the wrong file, silently. That is what happened when investmentValuationSvc and
// investmentEditSvc were added — opening a data file left both pointing at the
// previous database, so valuing an account reported "account not found" for an ID
// that plainly existed, and the dashboard, which swallows that error, simply
// stopped showing total return.
//
// Rather than assert the two fields that broke, this walks App by reflection and
// requires that EVERY field whose type also appears on *app.Services is a
// different pointer after the switch. A new service field is then caught by this
// test the day it is added, without anyone remembering to extend a list.
func TestSwitchDatabase_RepointsEveryService(t *testing.T) {
	first := dbtest.New(t)
	second := dbtest.New(t)

	a := NewApp(first, &config.Config{})

	// Every type that *app.Services hands out. A field on App of one of these
	// types came from a Services built for a specific database.
	serviceTypes := map[reflect.Type]bool{}
	svcT := reflect.TypeOf(app.Services{})
	for i := 0; i < svcT.NumField(); i++ {
		if f := svcT.Field(i); f.Type.Kind() == reflect.Pointer {
			serviceTypes[f.Type] = true
		}
	}

	before := map[string]uintptr{}
	appV := reflect.ValueOf(a).Elem()
	appT := appV.Type()
	for i := 0; i < appT.NumField(); i++ {
		f := appT.Field(i)
		if !serviceTypes[f.Type] {
			continue
		}
		v := appV.Field(i)
		if v.IsNil() {
			continue
		}
		before[f.Name] = v.Pointer()
	}
	if len(before) == 0 {
		t.Fatal("no service-typed fields found on App; the reflection rule is not matching anything")
	}

	if _, cmd := a.switchDatabase(second); cmd == nil {
		t.Fatal("switchDatabase returned no reload command")
	}

	var stale []string
	for i := 0; i < appT.NumField(); i++ {
		f := appT.Field(i)
		old, tracked := before[f.Name]
		if !tracked {
			continue
		}
		v := appV.Field(i)
		if v.IsNil() || v.Pointer() == old {
			stale = append(stale, f.Name)
		}
	}
	if len(stale) > 0 {
		t.Errorf("switchDatabase left %d service field(s) bound to the previous database: %v\n"+
			"Add the missing assignment(s) in switchDatabase — a field left behind reads and "+
			"writes the wrong file with no error the user can interpret.", len(stale), stale)
	}
}

// staleCommand stands in for any undo command: every real one captures the
// service it was built against, so after a database switch it points at a
// closed file. The test needs only the shape, not a real write.
type staleCommand struct{}

func (staleCommand) Execute() error      { return nil }
func (staleCommand) Undo() error         { return nil }
func (staleCommand) Description() string { return "stale" }

// TestSwitchDatabase_ClearsUndoHistory pins the other half of a file switch.
// Re-pointing the services is not enough: the commands already on the undo
// and redo stacks hold the OLD services, so Undo after a switch would write to
// the database switchDatabase just retired. The history has to go, as it does
// in reloadAfterRestore.
func TestSwitchDatabase_ClearsUndoHistory(t *testing.T) {
	first := dbtest.New(t)
	second := dbtest.New(t)

	a := NewApp(first, &config.Config{})
	if err := a.undoManager.Execute(staleCommand{}); err != nil {
		t.Fatal(err)
	}
	if err := a.undoManager.Execute(staleCommand{}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.undoManager.Undo(); err != nil {
		t.Fatal(err)
	}
	if a.undoManager.UndoLen() != 1 || a.undoManager.RedoLen() != 1 {
		t.Fatalf("setup: want 1 undo + 1 redo, got %d + %d", a.undoManager.UndoLen(), a.undoManager.RedoLen())
	}

	a.switchDatabase(second)

	if n, r := a.undoManager.UndoLen(), a.undoManager.RedoLen(); n != 0 || r != 0 {
		t.Errorf("switchDatabase left %d undo and %d redo command(s) built against the previous database", n, r)
	}
}
