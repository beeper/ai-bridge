package stringutil

// BoolPtrOr dereferences a *bool, returning fallback if the pointer is nil.
func BoolPtrOr(ptr *bool, fallback bool) bool {
	if ptr == nil {
		return fallback
	}
	return *ptr
}
