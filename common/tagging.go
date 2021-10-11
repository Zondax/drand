package common

type Tag struct {
	DecouplePrevSig bool
}

var Tags = map[string]Tag{"Pedersen-bls-chanined": {DecouplePrevSig: false}, "Pedersen-bls-unchanined": {DecouplePrevSig: true}}
