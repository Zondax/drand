package scheme

const DefaultSchemeID = "pedersen-bls-chanined"

type Scheme struct {
	ID              string
	DecouplePrevSig bool
}

var schemes = []Scheme{{ID: "pedersen-bls-chanined", DecouplePrevSig: false}, {ID: "pedersen-bls-unchanined", DecouplePrevSig: true}}

func GetSchemeByID(id string) (scheme Scheme, found bool) {
	for _, t := range schemes {
		if t.ID == id {
			return t, true
		}
	}

	return Scheme{}, false
}
