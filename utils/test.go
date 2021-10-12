package utils

import (
	"os"

	"github.com/drand/drand/common/scheme"
)

func SchemeFromEnv() (scheme.Scheme, bool) {
	id := os.Getenv("SCHEME_ID")
	if id == "" {
		id = scheme.DefaultSchemeID
	}

	return scheme.GetSchemeByID(id)
}

func SchemeForTesting() scheme.Scheme {
	sch, ok := SchemeFromEnv()
	if !ok {
		panic("scheme is not valid")
	}

	return sch
}
