package node

import (
	"reflect"

	"go.uber.org/fx"
)

// Option is a functional option which can be used with the New function to
// change how the node is constructed
//
// Options are applied in sequence
type Option func(*Settings) error

// Options groups multiple options into one
func Options(opts ...Option) Option {
	return func(s *Settings) error {
		for _, opt := range opts {
			if err := opt(s); err != nil {
				return err
			}
		}
		return nil
	}
}

// Error is a special option which returns an error when applied
func Error(err error) Option {
	return func(_ *Settings) error {
		return err
	}
}

func ApplyIf(check func(s *Settings) bool, opts ...Option) Option {
	return func(s *Settings) error {
		if check(s) {
			return Options(opts...)(s)
		}
		return nil
	}
}

func If(b bool, opts ...Option) Option {
	return ApplyIf(func(s *Settings) bool {
		return b
	}, opts...)
}

// Override option changes constructor for a given type
func Override(typ, constructor interface{}) Option {
	return func(s *Settings) error {
		if i, ok := typ.(invoke); ok {
			s.invokes[i] = fx.Invoke(constructor)
			return nil
		}

		if c, ok := typ.(special); ok {
			s.modules[c] = fx.Provide(constructor)
			return nil
		}
		ctor := as(constructor, typ)
		rt := reflect.TypeOf(typ).Elem()
		// We index `modules` with the type `typ` points to.
		s.modules[rt] = fx.Provide(ctor)
		return nil
	}
}

func Unset(typ interface{}) Option {
	return func(s *Settings) error {
		if i, ok := typ.(invoke); ok {
			s.invokes[i] = nil
			return nil
		}

		if c, ok := typ.(special); ok {
			delete(s.modules, c)
			return nil
		}
		rt := reflect.TypeOf(typ).Elem()

		delete(s.modules, rt)
		return nil
	}
}

// From(*T) -> func(t T) T {return t}
// The return type T will be overwritten in Override. The input is normally
// an `fx.In` structure that the DI system will populate.
// FIXME: Why do we need this if we already implicitly call `as` in `Override()`?
//  Can't we move this logic there?
func From(typ interface{}) interface{} {
	// FIXME: Validate input before unwrapping with Elem(), we usually expect a pointer.
	rt := []reflect.Type{reflect.TypeOf(typ).Elem()}
	ft := reflect.FuncOf(rt, rt, false)
	return reflect.MakeFunc(ft, func(args []reflect.Value) (results []reflect.Value) {
		return args
	}).Interface()
}

// from go-ipfs
// as casts input constructor to a given interface (if a value is given, it
// wraps it into a constructor).
//
// Note: this method may look like a hack, and in fact it is one.
// This is here only because https://github.com/uber-go/fx/issues/673 wasn't
// released yet
//
// Note 2: when making changes here, make sure this method stays at
// 100% coverage. This makes it less likely it will be terribly broken
func as(in interface{}, as interface{}) interface{} {
	// `as` is assumed to be a pointer in this case (`new()` -> `*Type).

	// FIXME: Do we actually need to call this every time? We should at least
	//  put a check here to see if types already match and do nothing.

	// FIXME: VERY CONFUSING. Renaming the `as` with the `out` prefix will
	//  be misleading when manipulating the rest of the outputs.
	outType := reflect.TypeOf(as)

	if outType.Kind() != reflect.Ptr {
		panic("outType is not a pointer")
	}

	if reflect.TypeOf(in).Kind() != reflect.Func {
		// FIXME: This should absorb the From case, distinguishing if we want a pointer
		//  or not. From() unwraps the pointer, this case doesn't.
		ctype := reflect.FuncOf(nil, []reflect.Type{outType.Elem()}, false)

		// Wrap the `in` in a function that returns it with type `as`.
		return reflect.MakeFunc(ctype, func(args []reflect.Value) (results []reflect.Value) {
			// First dereference the pointer, `Type.Elem()`. (Empty value pointer.)
			out := reflect.New(outType.Elem())
			// Then take the value pointed to, `Value.Elem()`. Set it to the input
			// function. So it's a pointer to a non-function? (Normally the value
			// that has been provided.)
			// FIXME: Check if settable.
			out.Elem().Set(reflect.ValueOf(in))

			return []reflect.Value{out.Elem()}
		}).Interface()
	}

	// `in` is a function, take its signature type, its inputs/outputs:
	inType := reflect.TypeOf(in)
	ins := make([]reflect.Type, inType.NumIn())
	outs := make([]reflect.Type, inType.NumOut())
	for i := range ins {
		ins[i] = inType.In(i)
	}
	// FIXME: What are we assuming here by extracting the element of the first
	//  return value? That it's a pointer? Yes, it is the `as` argument.
	// We will overwrite the first return value with the `as` type.
	outs[0] = outType.Elem()
	for i := range outs[1:] {
		outs[i+1] = inType.Out(i + 1)
		// FIXME: Change names, `inType.Out()` is confusing.
	}
	// The extracted types of the functions in/outs are use to reconstruct
	// the function type.
	// FIXME: What is the difference between ctype and the original inType
	//  from which it was constructed? Only the first return type.
	ctype := reflect.FuncOf(ins, outs, false)

	// Make a function with the constructed type calling the original in function
	// changing then the return type of the first element.
	// FIXME: Complete description.
	return reflect.MakeFunc(ctype, func(args []reflect.Value) (results []reflect.Value) {
		outs := reflect.ValueOf(in).Call(args)

		// We replace the first return value (leaving the rest of the `outs` as is).
		// FIXME: Again calling `Elem()` on the type "de-referencing" it.
		out := reflect.New(outType.Elem())
		// Using `out` as an intermediary for the assignment to the real return
		// values in `outs`.
		// If the original return value and the new (`as`) one are compatible
		// just change it.
		if outs[0].Type().AssignableTo(outType.Elem()) {
			// Out: Iface = In: *Struct; Out: Iface = In: OtherIface
			out.Elem().Set(outs[0])
		} else {
			// FIXME: Expand on this case.
			// Out: Iface = &(In: Struct)
			t := reflect.New(outs[0].Type())
			// FIXME: If we assume this is a pointer explicitly check it.
			t.Elem().Set(outs[0])
			out.Elem().Set(t)
			// FIXME: How do we know the type of the function return is compatible
			//  with the new type `as`? We already know `AssignableTo` is false.
			//  How are these two lines different from the other if case above
			//  `out.Elem().Set(outs[0])`?
			// The difference is the `t.Elem()` indirection. We don't assign
			//  outs[0] but something pointing to outs[0]. The fact the pointer
			//  is also of type `outs[0].Type()` seems confusing and it may also
			//  depend on other assumptions.
		}
		// FIXME: Explain why can't we just assign to outs[0] directly. (Because
		//  we seem to be covering two different cases in the `if`, this final
		//  assignment could have been absorbed there.)
		outs[0] = out.Elem()

		return outs
	}).Interface()
}
