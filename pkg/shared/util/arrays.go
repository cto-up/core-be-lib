package util

import "strings"

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

// containsAny checks if the main string contains any of the substrings in the list.
func ContainsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
