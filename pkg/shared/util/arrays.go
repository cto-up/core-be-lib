package util

func GetNotNilArray[K interface{}](array []K) []K {
	if array == nil {
		return []K{}
	}
	return array
}

func Contains[T comparable](arr []T, elem T) bool {
	for _, v := range arr {
		if v == elem {
			return true
		}
	}
	return false
}
