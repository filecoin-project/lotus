package must

func One[R any](r R, err error) R {
	if err != nil {
		panic(err)
	}

	return r
}
