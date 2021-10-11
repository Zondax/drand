package common

type ConfigPreset struct {
	Id              string
	DecouplePrevSig bool
}

var configPresets = []ConfigPreset{{Id: "Pedersen-bls-chanined", DecouplePrevSig: false}, {Id: "Pedersen-bls-unchanined", DecouplePrevSig: true}}

func GetConfigPresetById(id string) (tag ConfigPreset, found bool) {
	for _, t := range configPresets {
		if t.Id == id {
			return t, true
		}
	}

	return ConfigPreset{}, false
}
