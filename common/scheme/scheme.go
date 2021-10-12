package scheme

const DefaultSchemeID = "pedersen-bls-chained"

type Scheme struct {
	ID              string
	DecouplePrevSig bool
}

var schemes = []Scheme{{ID: "pedersen-bls-chained", DecouplePrevSig: false}, {ID: "pedersen-bls-unchained", DecouplePrevSig: true}}

func GetSchemeByID(id string) (scheme Scheme, found bool) {
	for _, t := range schemes {
		if t.ID == id {
			return t, true
		}
	}

	return Scheme{}, false
}
