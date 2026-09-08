package pinnermcp

import "reflect"

// isNilValue reports whether v is a nil interface or an interface holding a
// typed nil (nil pointer/map/slice/chan/func value). A plain `v == nil`
// comparison misses the typed-nil shape: an interface variable carrying a nil
// concrete value is non-nil as an interface yet its methods may be unsafe on
// a nil receiver.
func isNilValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}
