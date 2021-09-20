package core

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/stretchr/testify/require"
)

func setFDLimit() {
	fdOpen := 2000
	_, max, err := unixGetLimit()
	if err != nil {
		panic(err)
	}
	if err := unixSetLimit(uint64(fdOpen), max); err != nil {
		panic(err)
	}
}

// 1 second after end of dkg
var testBeaconOffset = 1
var testDkgTimeout = 2 * time.Second

// Test that a normal dkg process works correctly
func TestRunDKG(t *testing.T) {
	n := 4
	expectedBeaconPeriod := 5 * time.Second

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), expectedBeaconPeriod)
	defer dt.Cleanup()

	group := dt.RunDKG()

	t.Log(group)

	assert.Equal(t, 3, group.Threshold)
	assert.Equal(t, expectedBeaconPeriod, group.Period)
	assert.Equal(t, time.Duration(0), group.CatchupPeriod)
	assert.Equal(t, n, len(group.Nodes))
	assert.Equal(t, int64(449884810), group.GenesisTime)

	t.Log("DKG FINISHED")
}

// Test dkg for a large quantity of nodes
func TestRunDKGLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	setFDLimit()

	n := 22
	expectedBeaconPeriod := 5 * time.Second

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), expectedBeaconPeriod)
	defer dt.Cleanup()

	group := dt.RunDKG()

	assert.Equal(t, 12, group.Threshold)
	assert.Equal(t, expectedBeaconPeriod, group.Period)
	assert.Equal(t, time.Duration(0), group.CatchupPeriod)
	assert.Equal(t, n, len(group.Nodes))
	assert.Equal(t, int64(449884810), group.GenesisTime)

	t.Log("DKG FINISHED")
}

// Test ??????????????????????
func TestDrandDKGFresh(t *testing.T) {
	n := 4
	beaconPeriod := 1 * time.Second

	dt := NewDrandTestScenario(t, n, key.DefaultThreshold(n), beaconPeriod)
	defer dt.Cleanup()

	finalGroup := dt.RunDKG()

	// make the last node fail (stop)
	lastNodeAddr := dt.nodes[n-1].addr
	t.Logf("Stop last node %s", lastNodeAddr)
	dt.StopMockNode(lastNodeAddr, false)

	// move time to genesis
	dt.SetMockClock(t, finalGroup.GenesisTime)
	t.Logf("Time = %d", finalGroup.GenesisTime)

	// two = genesis + 1st round (happens at genesis)
	t.Log("Check Beacon Length")
	runningNodeAddresses := dt.Ids(n-1, false)
	dt.CheckBeaconLength(2, false, runningNodeAddresses...)

	t.Logf("Start last node %s", lastNodeAddr)
	dt.StartDrand(lastNodeAddr, true, false)

	// leave some room to do the catchup
	// Wait for catch up
	// time.Sleep(2 * time.Second)

	dt.AdvanceMockClock(t, beaconPeriod)

	t.Log("Check Beacon Length")
	runningNodeAddresses = dt.Ids(n, false)
	dt.CheckBeaconLength(3, false, runningNodeAddresses...)

	t.Log("Check Beacon Public")
	response := dt.CheckPublicBeacon(lastNodeAddr, false)
	assert.Equal(t, uint64(2), response.Round)
}

// Test ??????????????????????
func TestDrandDKGBroadcastDeny(t *testing.T) {
	n := 4
	thr := 3
	beaconPeriod := 1 * time.Second

	dt := NewDrandTestScenario(t, n, thr, beaconPeriod)
	defer dt.Cleanup()

	// close connection between a pair of nodes
	n1 := dt.nodes[1]
	n2 := dt.nodes[2]
	n1.drand.DenyBroadcastTo(n2.addr)
	n2.drand.DenyBroadcastTo(n1.addr)
	group1 := dt.RunDKG()
	dt.SetMockClock(t, group1.GenesisTime)
	dt.AdvanceMockClock(t, 1*time.Second)
	t.Log("DKG FINISHED")

	time.Sleep(200 * time.Millisecond)
	_, err := dt.RunReshare(t, n, 0, thr, 1*time.Second, false, false)
	require.NoError(t, err)
	t.Log("RESHARING FINISHED")
}

// Test ??????????????????????
func TestDrandReshareForce(t *testing.T) {
	oldN := 4
	oldThr := 3
	timeout := 1 * time.Second
	beaconPeriod := 2 * time.Second

	dt := NewDrandTestScenario(t, oldN, oldThr, beaconPeriod)
	defer dt.Cleanup()

	group1 := dt.RunDKG()

	// TODO: make sure all nodes had enough time to run their go routines to start the beacon handler - related to CI problems
	// time.Sleep(getSleepDuration())

	dt.SetMockClock(t, group1.GenesisTime)
	dt.AdvanceMockClock(t, 1*time.Second)

	// run the resharing
	var reshareWg sync.WaitGroup
	reshareWg.Add(1)
	go func() {
		defer reshareWg.Done()
		t.Log("[reshare] Start reshare")
		_, err := dt.RunReshare(t, oldN, 0, oldThr, timeout, false, true)
		require.NoError(t, err)
	}()
	reshareWg.Wait()

	// force
	t.Log("[reshare] Start again!")
	group3, err := dt.RunReshare(t, oldN, 0, oldThr, timeout, true, false)
	require.NoError(t, err)

	t.Log("[reshare] Move to response phase!")
	t.Logf("[reshare] Group: %s", group3)
}

// This tests when a node first signal his intention to participate into a
// resharing but is down right after  - he shouldn't be in the final group
func TestDrandDKGReshareAbsent(t *testing.T) {
	oldN := 3
	newN := 4
	oldThr := 2
	newThr := 3
	timeout := 1 * time.Second
	beaconPeriod := 2 * time.Second

	dt := NewDrandTestScenario(t, oldN, oldThr, beaconPeriod)
	defer dt.Cleanup()

	group1 := dt.RunDKG()
	// make sure all nodes had enough time to run their go routines to start the
	// beacon handler - related to CI problems
	time.Sleep(getSleepDuration())
	dt.SetMockClock(t, group1.GenesisTime)
	// move to genesis time - so nodes start to make a round
	// dt.AdvanceMockClock(t,offsetGenesis)
	// two = genesis + 1st round (happens at genesis)
	dt.CheckBeaconLength(2, false, dt.Ids(oldN, false)...)
	// so nodes think they are going forward with round 2
	dt.AdvanceMockClock(t, 1*time.Second)

	toAdd := newN - oldN
	dt.SetupNewNodes(toAdd)
	// we want to stop one node right after the group is created
	nodeToStop := 1
	leader := 0

	dt.nodes[leader].drand.setupCB = func(g *key.Group) {
		t.Log("STOPPING NODE 1")
		dt.nodes[nodeToStop].drand.Stop(context.Background())
		t.Log("STOPPING NODE 1 DONE")
	}

	t.Log("SETUP RESHARE DONE")
	newGroup, err := dt.RunReshare(t, oldN, toAdd, newThr, timeout, false, false, true)
	require.NoError(t, err)
	require.NotNil(t, newGroup)

	// the node that had stopped must not be in the group
	missingPublic := dt.nodes[nodeToStop].drand.priv.Public
	require.Nil(t, newGroup.Find(missingPublic), "missing public is found", missingPublic)
}

// Test ??????????????????????
func TestDrandDKGReshareTimeout(t *testing.T) {
	oldN := 3
	newN := 4
	oldThr := 2
	newThr := 3
	timeout := 1 * time.Second
	beaconPeriod := 2 * time.Second
	offline := 1

	dt := NewDrandTestScenario(t, oldN, oldThr, beaconPeriod)
	defer dt.Cleanup()

	group1 := dt.RunDKG()
	// make sure all nodes had enough time to run their go routines to start the
	// beacon handler - related to CI problems
	time.Sleep(getSleepDuration())
	dt.SetMockClock(t, group1.GenesisTime)
	// move to genesis time - so nodes start to make a round
	// dt.AdvanceMockClock(t,offsetGenesis)
	// two = genesis + 1st round (happens at genesis)
	dt.CheckBeaconLength(2, false, dt.Ids(oldN, false)...)
	// so nodes think they are going forward with round 2
	dt.AdvanceMockClock(t, 1*time.Second)

	// + offline makes sure t
	toKeep := oldN - offline
	toAdd := newN - toKeep
	dt.SetupNewNodes(toAdd)

	t.Log("SETUP RESHARE DONE")

	// run the resharing
	var doneReshare = make(chan *key.Group)
	go func() {
		t.Log("[reshare] Start reshare")
		group, err := dt.RunReshare(t, toKeep, toAdd, newThr, timeout, false, false)
		require.NoError(t, err)
		doneReshare <- group
	}()
	time.Sleep(3 * time.Second)

	t.Log("MOVE TO RESPONSE PHASE")
	dt.AdvanceMockClock(t, timeout)
	// at this point in time, nodes should have gotten all deals and send back
	// their responses to all nodes
	time.Sleep(getSleepDuration())

	t.Log("MOVE TO JUSTIFICATION PHASE")
	dt.AdvanceMockClock(t, timeout)
	// at this time, all nodes received the responses of each other nodes but
	// there is one node missing so they expect justifications
	time.Sleep(getSleepDuration())

	t.Log("MOVE TO FINISH PHASE")
	dt.AdvanceMockClock(t, timeout)
	time.Sleep(getSleepDuration())
	// at this time they received no justification from the missing node so he's
	// exlucded of the group and the dkg should finish
	// time.Sleep(10 * time.Second)

	var resharedGroup *key.Group
	select {
	case resharedGroup = <-doneReshare:
	case <-time.After(1 * time.Second):
		require.True(t, false)
	}

	t.Logf("RESHARED GROUP: %s", resharedGroup)
	target := resharedGroup.TransitionTime
	now := dt.Now().Unix()
	// get rounds from first node in the "old" group - since he's the leader for
	// the new group, he's alive
	lastBeacon := dt.CheckPublicBeacon(dt.Ids(1, false)[0], false)
	// move to the transition time period by period - do not skip potential
	// periods as to emulate the normal time behavior
	for now < target-1 {
		dt.AdvanceMockClock(t, beaconPeriod)
		lastBeacon = dt.CheckPublicBeacon(dt.Ids(1, false)[0], false)
		now = dt.Now().Unix()
	}
	// move to the transition time
	dt.SetMockClock(t, resharedGroup.TransitionTime)
	time.Sleep(getSleepDuration())
	// test that all nodes in the new group have generated a new beacon
	dt.CheckBeaconLength(int(lastBeacon.Round+1), true, dt.Ids(newN, true)...)
}

// Test ??????????????????????
func TestDrandResharePreempt(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping testing in CI environment")
	}

	oldN := 3
	newN := 3
	Thr := 2
	timeout := 1 * time.Second
	beaconPeriod := 2 * time.Second

	dt := NewDrandTestScenario(t, oldN, Thr, beaconPeriod)
	defer dt.Cleanup()

	group1 := dt.RunDKG()
	// make sure all nodes had enough time to run their go routines to start the
	// beacon handler - related to CI problems
	time.Sleep(getSleepDuration())
	dt.SetMockClock(t, group1.GenesisTime)
	// move to genesis time - so nodes start to make a round
	dt.CheckBeaconLength(2, false, dt.Ids(oldN, false)...)
	// so nodes think they are going forward with round 2
	dt.AdvanceMockClock(t, 1*time.Second)

	// first, the leader is going to start running a failed reshare:
	oldNode := dt.group.Find(dt.nodes[0].drand.priv.Public)
	if oldNode == nil {
		panic("Leader not found in old group")
	}

	// old root: oldNode.Index leater: leader.addr
	oldDone := make(chan error, 1)
	go func() {
		client, err := net.NewControlClient(dt.nodes[0].drand.opts.controlPort)
		require.NoError(t, err)
		_, err = client.InitReshareLeader(newN, Thr, timeout, 0, "unused secret", "", testBeaconOffset)
		// Done resharing
		if err == nil {
			panic("initial reshare should fail.")
		}
		oldDone <- err
	}()
	time.Sleep(100 * time.Millisecond)

	// run the resharing
	var doneReshare = make(chan *key.Group, 1)
	go func() {
		g, err := dt.RunReshare(t, oldN, 0, Thr, timeout, false, false)
		require.NoError(t, err)
		doneReshare <- g
	}()
	time.Sleep(time.Second)
	dt.AdvanceMockClock(t, time.Second)
	time.Sleep(time.Second)

	t.Log("MOVE TO RESPONSE PHASE")
	dt.AdvanceMockClock(t, timeout)
	// at this point in time, nodes should have gotten all deals and send back
	// their responses to all nodes
	time.Sleep(getSleepDuration())

	t.Log("MOVE TO JUSTIFICATION PHASE")
	dt.AdvanceMockClock(t, timeout)
	// at this time, all nodes received the responses of each other nodes but
	// there is one node missing so they expect justifications
	time.Sleep(getSleepDuration())

	t.Log("MOVE TO FINISH PHASE")
	dt.AdvanceMockClock(t, timeout)
	time.Sleep(getSleepDuration())

	// at this time they received no justification from the missing node so he's
	// exlucded of the group and the dkg should finish
	select {
	case <-doneReshare:
	case <-time.After(1 * time.Second):
		panic("expect dkg to have finished within one second")
	}
	select {
	case <-oldDone:
	case <-time.After(1 * time.Second):
		panic("expect aborted dkg to fail")
	}
	dt.CheckPublicBeacon(dt.Ids(1, false)[0], false)
}

// Check they all have same chain info
func TestDrandPublicChainInfo(t *testing.T) {
	n := 10
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second

	dt := NewDrandTestScenario(t, n, thr, p)
	defer dt.Cleanup()

	group := dt.RunDKG()
	ci := chain.NewChainInfo(group)
	cm := dt.nodes[0].drand.opts.certmanager
	client := NewGrpcClientFromCert(cm)

	for _, node := range dt.nodes {
		d := node.drand
		received, err := client.ChainInfo(d.priv.Public)
		require.NoError(t, err, fmt.Sprintf("addr %s", node.addr))
		require.True(t, ci.Equal(received))
	}

	for _, node := range dt.nodes {
		var found bool
		addr := node.addr
		public := node.drand.priv.Public
		for _, n := range group.Nodes {
			sameAddr := n.Address() == addr
			sameKey := n.Key.Equal(public.Key)
			sameTLS := n.IsTLS() == public.TLS
			if sameAddr && sameKey && sameTLS {
				found = true
				break
			}
		}
		require.True(t, found)
	}

	// rest := net.NewRestClientFromCertManager(cm)
	// restGroup, err := rest.Group(context.TODO(), dt.nodes[0].drand.priv.Public, &drand.GroupRequest{})
	// require.NoError(t, err)
	// received, err := key.GroupFromProto(restGroup)
	// require.NoError(t, err)
	// require.True(t, group.Equal(received))
}

// Test if the we can correctly fetch the rounds after a DKG using the
// PublicRand RPC call
func TestDrandPublicRand(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second

	dt := NewDrandTestScenario(t, n, thr, p)
	defer dt.Cleanup()

	group := dt.RunDKG()
	time.Sleep(getSleepDuration())
	root := dt.nodes[0].drand
	rootID := root.priv.Public

	dt.SetMockClock(t, group.GenesisTime)
	// do a few periods
	for i := 0; i < 3; i++ {
		dt.AdvanceMockClock(t, group.Period)
	}

	cm := root.opts.certmanager
	client := net.NewGrpcClientFromCertManager(cm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// get last round first
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)

	initRound := resp.Round + 1
	max := initRound + 4
	for i := initRound; i < max; i++ {
		dt.AdvanceMockClock(t, group.Period)
		req := new(drand.PublicRandRequest)
		req.Round = i
		resp, err := client.PublicRand(ctx, rootID, req)
		require.NoError(t, err)
		require.Equal(t, i, resp.Round)
		t.Logf("REQUEST ROUND %d GOT ROUND %d", i, resp.Round)
	}
}

// Test if the we can correctly fetch the rounds after a DKG using the
// PublicRandStream RPC call
// It also test the follow method call (it avoid redoing an expensive and long
// setup on CI to test both behaviors).
func TestDrandPublicStream(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second

	dt := NewDrandTestScenario(t, n, thr, p)
	defer dt.Cleanup()

	group := dt.RunDKG()
	time.Sleep(getSleepDuration())
	root := dt.nodes[0]
	rootID := root.drand.priv.Public

	dt.SetMockClock(t, group.GenesisTime)
	// do a few periods
	for i := 0; i < 3; i++ {
		dt.AdvanceMockClock(t, group.Period)
	}

	cm := root.drand.opts.certmanager
	client := net.NewGrpcClientFromCertManager(cm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// get last round first
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(t, err)

	//  run streaming and expect responses
	req := &drand.PublicRandRequest{Round: resp.GetRound()}
	respCh, err := client.PublicRandStream(ctx, root.drand.priv.Public, req)
	require.NoError(t, err)

	// expect first round now since node already has it
	select {
	case beacon := <-respCh:
		require.Equal(t, beacon.GetRound(), resp.GetRound())
	case <-time.After(100 * time.Millisecond):
		require.True(t, false, "too late")
	}
	nTry := 4
	// we expect the next one now
	initRound := resp.Round + 1
	maxRound := initRound + uint64(nTry)
	t.Logf("Streaming for future rounds starting from %d", initRound)

	for round := initRound; round < maxRound; round++ {
		// move time to next period
		dt.AdvanceMockClock(t, group.Period)
		select {
		case beacon := <-respCh:
			require.Equal(t, beacon.GetRound(), round)
		case <-time.After(1 * time.Second):
			require.True(t, false, "too late for streaming, round %d didn't reply in time", round)
		}
	}

	// try fetching with round 0 -> get latest
	respCh, err = client.PublicRandStream(ctx, root.drand.priv.Public, new(drand.PublicRandRequest))
	require.NoError(t, err)
	select {
	case <-respCh:
		require.False(t, true, "shouldn't get an entry if time doesn't go by")
	case <-time.After(50 * time.Millisecond):
		// correct
	}

	dt.AdvanceMockClock(t, group.Period)
	select {
	case resp := <-respCh:
		require.Equal(t, maxRound, resp.GetRound())
	case <-time.After(50 * time.Millisecond):
		// correct
	}
}

func TestDrandFollowChain(tt *testing.T) {
	n := 4
	p := 1 * time.Second

	dt := NewDrandTestScenario(tt, n, key.DefaultThreshold(n), p)
	defer dt.Cleanup()

	group := dt.RunDKG()
	time.Sleep(getSleepDuration())
	rootID := dt.nodes[0].drand.priv.Public

	dt.SetMockClock(tt, group.GenesisTime)
	// do a few periods
	for i := 0; i < 6; i++ {
		dt.AdvanceMockClock(tt, group.Period)
	}

	client := net.NewGrpcClientFromCertManager(dt.nodes[0].drand.opts.certmanager)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// get last round first
	resp, err := client.PublicRand(ctx, rootID, new(drand.PublicRandRequest))
	require.NoError(tt, err)

	// TEST setup a new node and fetch history

	newNode := dt.SetupNewNodes(1)[0]
	newClient, err := net.NewControlClient(newNode.drand.opts.controlPort)
	require.NoError(tt, err)
	addrToFollow := []string{rootID.Address()}
	hash := fmt.Sprintf("%x", chain.NewChainInfo(group).Hash())
	tls := true

	// First try with an invalid hash info
	ctx, cancel = context.WithCancel(context.Background())
	_, errCh, _ := newClient.StartFollowChain(ctx, "deadbeef", addrToFollow, tls, 10000)

	select {
	case <-errCh:
	case <-time.After(100 * time.Millisecond):
		panic("should have errored")
	}
	_, errCh, _ = newClient.StartFollowChain(ctx, "tutu", addrToFollow, tls, 10000)
	select {
	case <-errCh:
	case <-time.After(100 * time.Millisecond):
		panic("should have errored")
	}

	fn := func(upTo, exp uint64) {
		ctx, cancel = context.WithCancel(context.Background())
		progress, errCh, err := newClient.StartFollowChain(ctx, hash, addrToFollow, tls, upTo)
		require.NoError(tt, err)
		var goon = true
		for goon {
			select {
			case p, ok := <-progress:
				if ok && p.Current == exp {
					// success
					tt.Logf("SUCCESSFUL BEACON RCV. ROUND: %d. KEEP FOLLOWING CHAIN.", exp)
					goon = false
					break
				}
			case e := <-errCh:
				if e == io.EOF {
					break
				}
				require.NoError(tt, e)
			case <-time.After(1 * time.Second):
				tt.FailNow()
			}
		}
		// cancel the operation
		cancel()

		// check if the beacon is in the database
		store, err := newNode.drand.createBoltStore()
		require.NoError(tt, err)
		defer store.Close()

		lastB, err := store.Last()
		require.NoError(tt, err)
		require.Equal(tt, exp, lastB.Round, "found %d vs expected %d", lastB.Round, exp)
	}
	fn(resp.GetRound()-2, resp.GetRound()-2)
	time.Sleep(200 * time.Millisecond)
	fn(0, resp.GetRound())
}

// Test if the we can correctly fetch the rounds through the local proxy
func TestDrandPublicStreamProxy(t *testing.T) {
	n := 4
	thr := key.DefaultThreshold(n)
	p := 1 * time.Second

	dt := NewDrandTestScenario(t, n, thr, p)
	defer dt.Cleanup()

	group := dt.RunDKG()
	time.Sleep(getSleepDuration())

	root := dt.nodes[0]
	dt.SetMockClock(t, group.GenesisTime)

	// do a few periods
	for i := 0; i < 3; i++ {
		dt.AdvanceMockClock(t, group.Period)
	}

	client := &drandProxy{root.drand}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// get last round first
	resp, err := client.Get(ctx, 0)
	require.NoError(t, err)

	//  run streaming and expect responses
	rc := client.Watch(ctx)
	// expect first round now since node already has it
	dt.AdvanceMockClock(t, group.Period)
	beacon, ok := <-rc
	if !ok {
		panic("expected beacon")
	}
	require.Equal(t, beacon.Round(), resp.Round()+1)

	nTry := 4
	// we expect the next one now
	initRound := resp.Round() + 2
	maxRound := initRound + uint64(nTry)
	for round := initRound; round < maxRound; round++ {
		// move time to next period
		dt.AdvanceMockClock(t, group.Period)
		beacon, ok = <-rc
		require.True(t, ok)
		require.Equal(t, round, beacon.Round())
	}
}
