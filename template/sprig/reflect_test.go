package sprig

import (
	"testing"
)

type fixtureTO struct {
	Name, Value string
}

func TestTypeOf(t *testing.T) {
	f := &fixtureTO{"hello", "world"}
	tpl := `{{typeOf .}}`
	if err := runtv(tpl, "*sprig.fixtureTO", f); err != nil {
		t.Error(err)
	}
}

func TestKindOf(t *testing.T) {
	tpl := `{{kindOf .}}`

	f := fixtureTO{"hello", "world"}
	if err := runtv(tpl, "struct", f); err != nil {
		t.Error(err)
	}

	f2 := []string{"hello"}
	if err := runtv(tpl, "slice", f2); err != nil {
		t.Error(err)
	}

	var f3 *fixtureTO
	if err := runtv(tpl, "ptr", f3); err != nil {
		t.Error(err)
	}
}

func TestTypeIs(t *testing.T) {
	f := &fixtureTO{"hello", "world"}
	tpl := `{{if typeIs "*sprig.fixtureTO" .}}t{{else}}f{{end}}`
	if err := runtv(tpl, "t", f); err != nil {
		t.Error(err)
	}

	f2 := "hello"
	if err := runtv(tpl, "f", f2); err != nil {
		t.Error(err)
	}
}
func TestTypeIsLike(t *testing.T) {
	f := "foo"
	tpl := `{{if typeIsLike "string" .}}t{{else}}f{{end}}`
	if err := runtv(tpl, "t", f); err != nil {
		t.Error(err)
	}

	// Now make a pointer. Should still match.
	f2 := &f
	if err := runtv(tpl, "t", f2); err != nil {
		t.Error(err)
	}
}
func TestKindIs(t *testing.T) {
	f := &fixtureTO{"hello", "world"}
	tpl := `{{if kindIs "ptr" .}}t{{else}}f{{end}}`
	if err := runtv(tpl, "t", f); err != nil {
		t.Error(err)
	}
	f2 := "hello"
	if err := runtv(tpl, "f", f2); err != nil {
		t.Error(err)
	}
}
