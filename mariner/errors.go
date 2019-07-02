package mariner

type cwlError struct {
	err     error
	context string
}

func (e *cwlError) Error() string {
	return e.context + ": " + e.err.Error()
}

// ParseError returns a workflow parse error
func ParseError(err error) error {
	return &cwlError{err, "Fail to parse workflow"}
}
