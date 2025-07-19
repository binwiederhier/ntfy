package sprig

import (
	"fmt"
	"reflect"
)

// typeIs returns true if the src is the type named in target.
func typeIs(target string, src any) bool {
	return target == typeOf(src)
}

func typeIsLike(target string, src any) bool {
	t := typeOf(src)
	return target == t || "*"+target == t
}

func typeOf(src any) string {
	return fmt.Sprintf("%T", src)
}

func kindIs(target string, src any) bool {
	return target == kindOf(src)
}

func kindOf(src any) string {
	return reflect.ValueOf(src).Kind().String()
}
