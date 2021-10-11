package utils

import (
	"os"

	"github.com/drand/drand/common/scheme"
)

func SchemeForTesting() (scheme.Scheme, bool) {
	id := os.Getenv("SCHEME_ID")
	if id == "" {
		id = scheme.DefaultSchemeId
	}

	return scheme.GetSchemeById(id)
}
