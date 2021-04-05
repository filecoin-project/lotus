package api

import "reflect"

// Wrap adapts partial api impl to another version
// proxyT is the proxy type used as input in wrapperT
// Usage: Wrap(new(v1api.FullNodeStruct), new(v0api.WrapperV1Full), eventsApi).(EventAPI)
func Wrap(proxyT, wrapperT, impl interface{}) interface{} {
	proxy := reflect.New(reflect.TypeOf(proxyT).Elem())
	proxyMethods := proxy.FieldByName("Internal")
	ri := reflect.ValueOf(impl)

	for i := 0; i < ri.NumMethod(); i++ {
		mt := ri.Type().Method(i)
		proxyMethods.FieldByName(mt.Name).Set(ri.Method(i))
	}

	wp := reflect.New(reflect.TypeOf(wrapperT).Elem())
	wp.Field(0).Set(proxy)
	return wp.Interface()
}
