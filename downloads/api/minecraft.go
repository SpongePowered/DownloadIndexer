package api

import (
	"strconv"
	"strings"
)

type minecraftVersions []string

func (versions minecraftVersions) Len() int {
	return len(versions)
}

func (versions minecraftVersions) Less(i, j int) bool {
	a := strings.Split(versions[i], ".")
	b := strings.Split(versions[j], ".")

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

func (versions minecraftVersions) Swap(i, j int) {
	versions[i], versions[j] = versions[j], versions[i]
}
