package utils

import (
	"os"

	"github.com/drand/drand/common/scheme"
)

func SchemeFromEnv() (scheme.Scheme, bool) {
	id := os.Getenv("SCHEME_ID")
	if id == "" {
		id = scheme.DefaultSchemeId
	}

	return scheme.GetSchemeById(id)
}

func SchemeForTesting() scheme.Scheme {
	scheme, ok := SchemeFromEnv()
	if !ok {
		panic("scheme is not valid")
	}

	return scheme
}
