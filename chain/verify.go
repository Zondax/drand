package chain

import (
	"crypto/sha256"

	"github.com/drand/drand/key"
	"github.com/drand/kyber"
)

// Message returns a slice of bytes as the message to sign or to verify
// alongside a beacon signature.
// H ( currRound)
func MessageUnchained(currRound uint64, _ []byte) []byte {
	h := sha256.New()
	_, _ = h.Write(RoundToBytes(currRound))
	return h.Sum(nil)
}

// MessageChained returns a slice of bytes as the message to sign or to verify
// alongside a beacon signature.
// H ( prevSig || currRound)
func MessageChained(currRound uint64, prevSig []byte) []byte {
	h := sha256.New()
	_, _ = h.Write(prevSig)
	_, _ = h.Write(RoundToBytes(currRound))
	return h.Sum(nil)
}

// VerifyChainedBeacon returns an error if the given beacon does not verify given the
// public key. The public key "point" can be obtained from the
// `key.DistPublic.Key()` method. The distributed public is the one written in
// the configuration file of the network.
func VerifyChainedBeacon(b Beacon, pubkey kyber.Point) error {
	prevSig := b.PreviousSig
	round := b.Round
	msg := MessageChained(round, prevSig)

	return key.Scheme.VerifyRecovered(pubkey, msg, b.Signature)
}

// VerifyUnchainedBeacon returns an error if the given beacon does not verify given the
// public key. The public key "point" can be obtained from the
// `key.DistPublic.Key()` method. The distributed public is the one written in
// the configuration file of the network.
func VerifyUnchainedBeacon(b Beacon, pubkey kyber.Point) error {
	prevSig := b.PreviousSig
	round := b.Round
	msg := MessageUnchained(round, prevSig)

	return key.Scheme.VerifyRecovered(pubkey, msg, b.Signature)
}
