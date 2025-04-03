package testutils

import (
	"math/rand"
	"strings"
	"time"
)

var alphabet = "azertyuiopqsdfghjklmwxcvbn"
var currencies = []string{"EUR", "USD", "VND"}

var interests = []string{"technologies", "green", "well-being"}

func init() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
}

func Random(min, max int64) int64 {
	return min + rand.Int63n(max-min+1)
}
func RandomString(length int64) string {
	var sb strings.Builder
	k := len(alphabet)
	for i := 0; i < int(length); i++ {
		b := alphabet[rand.Intn(k)]
		sb.WriteByte(b)
	}
	return sb.String()
}
func RandomOwner() string {
	return RandomString(28)
}

func RandomTenant() string {
	return RandomString(10)
}

func RandomAbout() string {
	return RandomString(500)
}
func RandomInterests(min, max int64) []string {
	k := len(interests)

	randomInterests := make([]string, max-min)

	for i := min; i < max; i++ {
		randomInterests = append(randomInterests, currencies[rand.Intn(k)])
	}
	return randomInterests
}
