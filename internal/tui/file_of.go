package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/db"
)

// fileOf tags the message of a load with the handle of the file that the load
// read. A load runs in its own goroutine, so a File > Open can switch the file
// before its message comes back. Update then drops the message (see
// fromAnotherFile): the view of the new file would show the data, or the
// error, of the old one. A message that is not tagged applies as it is.
//
// A load embeds fileOf in its message, and in the errMsg of its failure. A
// write embeds it in the message of its success, whose Update arm reloads or
// changes the view of the open file; the failure of a write is not tagged, so
// that it shows also after a switch. The tag is set from a.db when the
// command is made, never when it runs.
//
// A dropped success shows no confirmation. That is on purpose: the write is
// done, in its own file, and only the view of that file shows it.
type fileOf struct{ db *db.DB }

// loadedFrom returns the handle that the load read, or nil when the message
// is not tagged.
func (f fileOf) loadedFrom() *db.DB { return f.db }

// fromFile is a message that can carry the handle of its file.
type fromFile interface{ loadedFrom() *db.DB }

// fileOfOpen returns the tag of the file that is open now.
func (a *App) fileOfOpen() fileOf { return fileOf{db: a.db} }

// fromAnotherFile reports whether msg is the result of a load on a file that
// is no longer the open one.
func (a *App) fromAnotherFile(msg tea.Msg) bool {
	m, ok := msg.(fromFile)
	return ok && m.loadedFrom() != nil && m.loadedFrom() != a.db
}
