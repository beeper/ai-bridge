package cron

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func ptrInt64(v int64) *int64 {
	return &v
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
