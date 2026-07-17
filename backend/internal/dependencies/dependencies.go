package dependencies

import "reflect"

// IsNil reports whether value is nil, including an interface containing a
// typed nil. It is intended for validating dependency graphs at construction
// boundaries.
func IsNil(value any) bool {
	if value == nil {
		return true
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
