package chain

import (
	"testing"

	"github.com/drand/drand/key"
	"github.com/drand/drand/utils"
	"github.com/drand/kyber/util/random"
)

func BenchmarkVerifyBeacon(b *testing.B) {
	secret := key.KeyGroup.Scalar().Pick(random.New())
	public := key.KeyGroup.Point().Mul(secret, nil)

	var round uint64 = 16
	prevSig := []byte("My Sweet Previous Signature")

	var msg []byte
	if utils.PrevSigDecoupling() {
		msg = MessageUnchained(round, prevSig)
	} else {
		msg = MessageChained(round, prevSig)
	}

	sig, _ := key.AuthScheme.Sign(secret, msg)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b := Beacon{
			PreviousSig: prevSig,
			Round:       16,
			Signature:   sig,
		}

		var err error
		if utils.PrevSigDecoupling() {
			err = VerifyUnchainedBeacon(b, public)
		} else {
			err = VerifyChainedBeacon(b, public)
		}

		if err != nil {
			panic(err)
		}
	}
}
