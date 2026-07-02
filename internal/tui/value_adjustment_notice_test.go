package tui

import (
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/dbtest"
)

func TestNewApp_ValueAdjustmentCollisionSurfacesNotice(t *testing.T) {
	// Isolate the applog write + config save triggered by the notice.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	database := dbtest.New(t)
	catSvc := category.NewService(category.NewRepository(database), database)
	if err := catSvc.Create(category.NewCategory(category.ValueAdjustmentCategoryName, category.TypeExpense)); err != nil {
		t.Fatalf("setup: create user category: %v", err)
	}

	cfg := &config.Config{}
	a := NewApp(database, cfg)

	toast := a.statusbar.Toast()
	if toast == nil {
		t.Fatal("expected a status-bar toast for the Value Adjustment collision")
	}
	if !strings.Contains(toast.Text, category.ValueAdjustmentCategoryName) {
		t.Errorf("toast text = %q, want it to mention %q", toast.Text, category.ValueAdjustmentCategoryName)
	}
	if !cfg.ValueAdjustmentNoticeShown {
		t.Error("ValueAdjustmentNoticeShown should be set so the notice fires only once")
	}
}

func TestNewApp_NoCollisionNoNotice(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Fresh DB: EnsureValueAdjustmentCategory seeds the system category
	// with no user collision.
	database := dbtest.New(t)

	cfg := &config.Config{}
	a := NewApp(database, cfg)

	if a.statusbar.Toast() != nil {
		t.Errorf("expected no toast when there is no collision, got %q", a.statusbar.Toast().Text)
	}
	if cfg.ValueAdjustmentNoticeShown {
		t.Error("ValueAdjustmentNoticeShown should stay false without a collision")
	}
}

func TestNewApp_CollisionNoticeSuppressedWhenAlreadyShown(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	database := dbtest.New(t)
	catSvc := category.NewService(category.NewRepository(database), database)
	if err := catSvc.Create(category.NewCategory(category.ValueAdjustmentCategoryName, category.TypeExpense)); err != nil {
		t.Fatalf("setup: create user category: %v", err)
	}

	// The notice was already shown in a prior session.
	cfg := &config.Config{ValueAdjustmentNoticeShown: true}
	a := NewApp(database, cfg)

	if a.statusbar.Toast() != nil {
		t.Errorf("notice should be suppressed once shown, got toast %q", a.statusbar.Toast().Text)
	}
}
