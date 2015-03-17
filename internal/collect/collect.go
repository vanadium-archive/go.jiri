package collect

// Error provides a mechanism for collecting errors from deferred
// functions. The mechanism executes all deferred functions
// irrespective of their return value, returning the first error it
// encounters or nil if no error is encountered.
func Error(fn func() error, err *error) {
	if *err != nil {
		fn()
	} else {
		*err = fn()
	}
}

// Errors provides a mechanism for collecting errors from deferred
// functions. The mechanism executes all deferred functions
// irrespective of their return value, collecting all the errors it
// encounters.
func Errors(fn func() error, errs *[]error) {
	if err := fn(); err != nil {
		*errs = append(*errs, err)
	}
}
