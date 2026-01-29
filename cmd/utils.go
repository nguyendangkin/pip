package cmd

import (
	"strings"
)

func ensureChinExtension(path string) string {
	if strings.HasSuffix(strings.ToLower(path), ".chin") {
		return path
	}
	return path + ".chin"
}
