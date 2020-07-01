package parmap

import (
	"reflect"
	"sync"
)

// MapArr transforms map into slice of map values
func MapArr(in interface{}) interface{} {
	rin := reflect.ValueOf(in)
	rout := reflect.MakeSlice(reflect.SliceOf(rin.Type().Elem()), rin.Len(), rin.Len())
	var i int

	it := rin.MapRange()
	for it.Next() {
		rout.Index(i).Set(it.Value())
		i++
	}

	return rout.Interface()
}

// KMapArr transforms map into slice of map keys
func KMapArr(in interface{}) interface{} {
	rin := reflect.ValueOf(in)
	rout := reflect.MakeSlice(reflect.SliceOf(rin.Type().Key()), rin.Len(), rin.Len())
	var i int

	it := rin.MapRange()
	for it.Next() {
		rout.Index(i).Set(it.Key())
		i++
	}

	return rout.Interface()
}

// KVMapArr transforms map into slice of functions returning (key, val) pairs.
// map[A]B => []func()(A, B)
func KVMapArr(in interface{}) interface{} {
	rin := reflect.ValueOf(in)

	t := reflect.FuncOf([]reflect.Type{}, []reflect.Type{
		rin.Type().Key(),
		rin.Type().Elem(),
	}, false)

	rout := reflect.MakeSlice(reflect.SliceOf(t), rin.Len(), rin.Len())
	var i int

	it := rin.MapRange()
	for it.Next() {
		k := it.Key()
		v := it.Value()

		rout.Index(i).Set(reflect.MakeFunc(t, func(args []reflect.Value) (results []reflect.Value) {
			return []reflect.Value{k, v}
		}))
		i++
	}

	return rout.Interface()
}

func Par(concurrency int, arr interface{}, f interface{}) {
	throttle := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	varr := reflect.ValueOf(arr)
	l := varr.Len()

	rf := reflect.ValueOf(f)

	wg.Add(l)
	for i := 0; i < l; i++ {
		throttle <- struct{}{}

		go func(i int) {
			defer wg.Done()
			defer func() {
				<-throttle
			}()
			rf.Call([]reflect.Value{varr.Index(i)})
		}(i)
	}

	wg.Wait()
}
