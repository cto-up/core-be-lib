package util

import (
	"regexp"
)

type Claim map[string]interface{}

func UppercaseOnly(key string, value interface{}) bool {
	regex := regexp.MustCompile("^[A-Z]")
	firstLetterUppercase := regex.MatchString(key)
	if !firstLetterUppercase {
		return false
	}
	// Assuming 'value' is of type bool in this context
	return value.(bool)
}

func FilterMapToArray(object map[string]interface{}, predicate func(key string, value interface{}) bool) []string {
	result := make([]string, 0)

	for key, value := range object {
		if predicate(key, value) {
			result = append(result, key)
		}
	}

	return result
}
