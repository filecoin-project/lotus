package api

import "reflect"

var _internalField = "Internal"

// GetInternalStructs extracts all pointers to 'Internal' sub-structs from the provided pointer to a proxy struct
func GetInternalStructs(in interface{}) []interface{} {
	return getInternalStructs(reflect.ValueOf(in).Elem())
}

func getInternalStructs(rv reflect.Value) []interface{} {
	var out []interface{}

	internal := rv.FieldByName(_internalField)
	ii := internal.Addr().Interface()
	out = append(out, ii)

	for i := 0; i < rv.NumField(); i++ {
		if rv.Type().Field(i).Name == _internalField {
			continue
		}

		sub := getInternalStructs(rv.Field(i))

		out = append(out, sub...)
	}

	return out
}
