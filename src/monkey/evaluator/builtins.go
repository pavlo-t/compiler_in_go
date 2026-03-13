package evaluator

import (
	"monkey/object"
)

var builtins = func() map[string]*object.Builtin {
	result := make(map[string]*object.Builtin, len(object.Builtins))
	for _, b := range object.Builtins {
		result[b.Name] = b.Builtin
	}
	return result
}()
