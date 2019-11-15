package main

import (
	"reflect"
	"sync"
)

func par(concurrency int, arr interface{}, f interface{}) {
	throttle := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	varr := reflect.ValueOf(arr)
	l := varr.Len()

	rf := reflect.ValueOf(f)

	wg.Add(l)
	for i := 0; i < l; i++ {
		throttle <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() {
				<-throttle
			}()
			rf.Call([]reflect.Value{varr.Index(i)})
		}()
	}
}
