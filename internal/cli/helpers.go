package cli

import (
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/db"
)

// openServices is a package-cli shim delegating to cmdutil.OpenServices so the
// 49 command files that call it keep compiling unchanged during the split.
func openServices(file string) (*db.DB, *app.Services, error) {
	return cmdutil.OpenServices(file)
}

// autoBackupAfterModification is a package-cli shim delegating to
// cmdutil.AutoBackupAfterModification.
func autoBackupAfterModification(dbPath string) {
	cmdutil.AutoBackupAfterModification(dbPath)
}
