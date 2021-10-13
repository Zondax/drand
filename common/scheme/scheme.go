package scheme

import "os"

const DefaultSchemeID = "pedersen-bls-chained"
const UnchainedSchemeID = "pedersen-bls-unchained"

type Scheme struct {
	ID              string
	DecouplePrevSig bool
}

var schemes = []Scheme{{ID: DefaultSchemeID, DecouplePrevSig: false}, {ID: UnchainedSchemeID, DecouplePrevSig: true}}

func GetSchemeByID(id string) (scheme Scheme, found bool) {
	for _, t := range schemes {
		if t.ID == id {
			return t, true
		}
	}

	return Scheme{}, false
}

func ListSchemes() (schemeIDs []string) {
	for _, t := range schemes {
		schemeIDs = append(schemeIDs, t.ID)
	}

	return schemeIDs
}

func ReadSchemeByEnv() (Scheme, bool) {
	id := os.Getenv("SCHEME_ID")
	if id == "" {
		id = DefaultSchemeID
	}

	return GetSchemeByID(id)
}

func GetSchemeFromEnv() Scheme {
	sch, ok := ReadSchemeByEnv()
	if !ok {
		panic("scheme is not valid")
	}

	return sch
}
