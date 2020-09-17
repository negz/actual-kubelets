package pointer

// DerefBoolOr dereferences and returns the supplied pointer. If the pointer is
// nil, it returns the supplied default value.
func DerefBoolOr(b *bool, dflt bool) bool {
	if b == nil {
		return dflt
	}
	return *b
}

// Int64OrNil returns a pointer to int64. If the supplied integer is zero, it
// returns nil.
func Int64OrNil(i int) *int64 {
	if i == 0 {
		return nil
	}
	i64 := int64(i)
	return &i64
}

// Bool returns a pointer to the supplied boolean.
func Bool(b bool) *bool {
	return &b
}
