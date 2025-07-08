package sprig

import (
	"fmt"
	"os"
	"text/template"
)

func Example() {
	// Set up variables and template.
	vars := map[string]interface{}{"Name": "  John Jacob Jingleheimer Schmidt "}
	tpl := `Hello {{.Name | trim | lower}}`

	// Get the Sprig function map.
	fmap := TxtFuncMap()
	t := template.Must(template.New("test").Funcs(fmap).Parse(tpl))

	err := t.Execute(os.Stdout, vars)
	if err != nil {
		fmt.Printf("Error during template execution: %s", err)
		return
	}
	// Output:
	// Hello john jacob jingleheimer schmidt
}
