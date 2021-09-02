package client_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/drand/drand/client"
	"github.com/drand/drand/client/test/result/mock"
)

func mockClientWithVerifiableResults(n int, decouplePrevSig bool) (client.Client, []mock.Result, error) {
	info, results := mock.VerifiableResults(n, decouplePrevSig)
	mc := client.MockClient{Results: results, StrictRounds: true}

	var c client.Client
	var err error
	if decouplePrevSig {
		c, err = client.Wrap(
			[]client.Client{client.MockClientWithInfo(info), &mc},
			client.WithChainInfo(info),
			client.WithVerifiedResult(&results[0]),
			client.WithFullChainVerification(),
			client.DecouplePrevSig(),
		)
	} else {
		c, err = client.Wrap(
			[]client.Client{client.MockClientWithInfo(info), &mc},
			client.WithChainInfo(info),
			client.WithVerifiedResult(&results[0]),
			client.WithFullChainVerification(),
		)
	}

	if err != nil {
		return nil, nil, err
	}
	return c, results, nil
}

func TestVerify(t *testing.T) {
	matrix := [2]bool{false, true}

	for _, decouplePrevSig := range matrix {
		c, results, err := mockClientWithVerifiableResults(3, decouplePrevSig)
		if err != nil {
			t.Fatal(err)
		}
		res, err := c.Get(context.Background(), results[1].Round())
		if err != nil {
			t.Fatal(err)
		}
		if res.Round() != results[1].Round() {
			t.Fatal("expected to get result.", results[1].Round(), res.Round(), fmt.Sprintf("%v", c))
		}
	}
}

func TestVerifyWithOldVerifiedResult(t *testing.T) {
	matrix := [2]bool{false, true}
	for _, decouplePrevSig := range matrix {
		c, results, err := mockClientWithVerifiableResults(5, decouplePrevSig)
		if err != nil {
			t.Fatal(err)
		}
		// should automatically load rounds 1, 2 and 3 to verify 4
		res, err := c.Get(context.Background(), results[4].Round())
		if err != nil {
			t.Fatal(err)
		}
		if res.Round() != results[4].Round() {
			t.Fatal("expected to get result.", results[4].Round(), res.Round(), fmt.Sprintf("%v", c))
		}
	}
}
