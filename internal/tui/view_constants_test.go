package tui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

// viewConstant is one View constant read from the const block in app.go.
type viewConstant struct {
	Name  string
	Value View
}

// viewConstants returns every View constant declared in app.go, in
// declaration order. A test that must cover every view ranges over this
// instead of a hand-written list, so a new constant is covered the moment it
// is declared. The view-table guards (VL-101) reuse it.
func viewConstants(t *testing.T) []viewConstant {
	t.Helper()
	src, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatalf("read app.go: %v", err)
	}
	consts, err := viewConstantsFromSource(src)
	if err != nil {
		t.Fatalf("enumerate View constants: %v", err)
	}
	if len(consts) == 0 {
		t.Fatal("found no View constants in app.go, so a test over them would pass vacuously")
	}
	return consts
}

// viewConstantsFromSource finds the const block whose first spec is
// `X View = iota` and returns its names. The value of each name is its index,
// which holds only while every later spec repeats the iota expression
// implicitly; any spec with its own type or value is an error rather than a
// wrong value.
func viewConstantsFromSource(src []byte) ([]viewConstant, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "app.go", src, 0)
	if err != nil {
		return nil, err
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST || !isViewIotaBlock(gen) {
			continue
		}
		var out []viewConstant
		for i, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			if i > 0 && (vs.Type != nil || len(vs.Values) > 0) {
				return nil, fmt.Errorf("View constant %s has its own type or value; "+
					"the enumerator assumes a plain iota block", vs.Names[0].Name)
			}
			for _, name := range vs.Names {
				if name.Name == "_" {
					return nil, fmt.Errorf("the View block skips a value with _; " +
						"the enumerator assumes one name per value")
				}
				out = append(out, viewConstant{Name: name.Name, Value: View(len(out))})
			}
		}
		return out, nil
	}
	return nil, nil
}

// isViewIotaBlock reports whether the block's first spec is `X View = iota`.
func isViewIotaBlock(gen *ast.GenDecl) bool {
	if len(gen.Specs) == 0 {
		return false
	}
	first := gen.Specs[0].(*ast.ValueSpec)
	typ, ok := first.Type.(*ast.Ident)
	if !ok || typ.Name != "View" || len(first.Values) != 1 {
		return false
	}
	val, ok := first.Values[0].(*ast.Ident)
	return ok && val.Name == "iota"
}

func TestViewConstantsFromSource(t *testing.T) {
	t.Run("reads an iota block in order", func(t *testing.T) {
		src := []byte(`package tui
type View int
const other = 1
const (
	ViewA View = iota
	ViewB
	ViewC
)`)
		got, err := viewConstantsFromSource(src)
		if err != nil {
			t.Fatal(err)
		}
		want := []viewConstant{{"ViewA", 0}, {"ViewB", 1}, {"ViewC", 2}}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got[%d] = %v, want %v", i, got[i], want[i])
			}
		}
	})

	t.Run("refuses an explicit value", func(t *testing.T) {
		src := []byte(`package tui
type View int
const (
	ViewA View = iota
	ViewB View = 7
)`)
		if _, err := viewConstantsFromSource(src); err == nil {
			t.Error("expected an error for a spec with its own value")
		}
	})

	t.Run("refuses a skipped value", func(t *testing.T) {
		src := []byte(`package tui
type View int
const (
	ViewA View = iota
	_
	ViewC
)`)
		if _, err := viewConstantsFromSource(src); err == nil {
			t.Error("expected an error for a blank name")
		}
	})

	t.Run("finds nothing without a View block", func(t *testing.T) {
		got, err := viewConstantsFromSource([]byte("package tui\nconst x = iota\n"))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("got %v, want none", got)
		}
	})
}

// TestViewConstants_MatchViewString ties the enumerator to the real
// constants: every value it returns has a name other than the "Unknown"
// fallback.
func TestViewConstants_MatchViewString(t *testing.T) {
	for _, vc := range viewConstants(t) {
		if vc.Value.String() == "Unknown" {
			t.Errorf("%s = %d, but View(%d).String() is Unknown; the enumerator and app.go disagree",
				vc.Name, vc.Value, vc.Value)
		}
	}
}
