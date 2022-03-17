package autoupdate

func toStrSet(sl []string) map[string]struct{} {
	result := make(map[string]struct{}, len(sl))

	for _, elem := range sl {
		result[elem] = struct{}{}
	}

	return result
}

func strSetToSlice(m map[string]struct{}) []string {
	res := make([]string, 0, len(m))

	for k := range m {
		res = append(res, k)
	}

	return res
}
