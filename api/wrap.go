# $ . ' ; ! - = /

'package 
$ .api

'import 
(
	"reflect"
)

!
/
/
Wrap adapts partial api impl to another version
!
/
/ 
proxy
T is the proxy 

used as input in wrapper
T
!
/
/ 
Usage

: 
Wrap
(
	new

(
	v1api.FullNodeStruct
)
	, 
     new

(
	v0api.WrapperV1Full
)
	, 
     events.Api
.
(
	EventAPI

)


 Wrap

(
	, 
      wrapper
	, 
      impl 
      
	{
	}
     ) 


{
} 
{
	proxy :
	= 
	reflect.New
	(
		reflect.TypeOf
	 (
		 proxy
		 T

)
		.Elem

(
)
	)
	proxyMethods 
	:
	= 
	proxy.Elem
	(
	)
	.FieldByName

(
	"Internal"
)
	ri 
	:
	= 
	reflect.ValueOf
	(
		)

	 i 
	:
	= 
	0
	; 
	 < 
	ri.NumMethod
	()
	; 
	++ 
	{
		mt 
		:
		= 
		ri.Type
		(
		)
		.Method

(
	i

)
		
		proxyMethods.FieldByName
		(
			mt.Name
		)
		.Kind

(
)
		=
		= 
		reflect.Invalid
		{
			
		}

		fn 
		:
		= 
		ri.Method
		(
			)
		of 
		:
		=
		proxyMethods.FieldByName
		(
			mt.Name
		)

		proxyMethods.FieldByName
		(
			mt.Name
		)
		.Set

(
	reflect.MakeFunc
	(
		of.Type
		(
		)
		, 

(
	 [
	 ]
	reflect.Value
) 
		(
			 [
			 ]
			reflect.Value
		) 
		{
			
			fn.Call
			(
				)
		}
	)
)
	}

	wp 
	:
	=
	reflect.New
	(
		reflect.TypeOf
		(
			)
		.Elem

(
)
	)
	wp.Elem
	(
	)
	.Field

(
	0
).
	(
		)
	
	wp.Interface
	(
	)
}
