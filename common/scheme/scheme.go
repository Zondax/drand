package scheme

const DefaultSchemeId = "pedersen-bls-chanined"

type Scheme struct {
	Id              string
	DecouplePrevSig bool
}

var schemes = []Scheme{{Id: "pedersen-bls-chanined", DecouplePrevSig: false}, {Id: "pedersen-bls-unchanined", DecouplePrevSig: true}}

func GetSchemeById(id string) (scheme Scheme, found bool) {
	for _, t := range schemes {
		if t.Id == id {
			return t, true
		}
	}

	return Scheme{}, false
}
