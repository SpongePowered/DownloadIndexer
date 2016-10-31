package api

import (
	"strconv"
	"strings"
)

type versions []string

func (v versions) Len() int {
	return len(v)
}

func (v versions) Less(i, j int) bool {
	a := strings.Split(v[i], ".")
	b := strings.Split(v[j], ".")

	max := len(a)
	if max > len(b) {
		max = len(b)
	}

	for i := 0; i < max; i++ {
		ai, _ := strconv.Atoi(a[i])
		bi, _ := strconv.Atoi(b[i])
		switch {
		case ai > bi:
			return true
		case bi > ai:
			return false
		}
	}

	return len(a) > len(b)
}

func (v versions) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}
