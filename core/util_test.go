package core

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	gnet "net"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/drand/drand/common/scheme"

	"github.com/kabukky/httpscerts"
	"github.com/stretchr/testify/assert"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
)

const (
	ReshareUnlock = iota
	ReshareLock
)

type MockNode struct {
	addr     string
	certPath string
	drand    *Drand
	clock    clock.FakeClock
}

type DrandTestScenario struct {
	sync.Mutex

	// note: do we need this here?
	t *testing.T

	// tmp dir for certificates, keys etc
	dir    string
	newDir string

	certPaths    []string
	newCertPaths []string
	// global clock on which all drand clocks are synchronized
	clock clock.FakeClock

	n             int
	thr           int
	newN          int
	newThr        int
	period        time.Duration
	catchupPeriod time.Duration
	scheme        scheme.Scheme

	// only set after the DKG
	group *key.Group
	// needed to give the group to new nodes during a resharing - only set after
	// a successful DKG
	groupPath string
	// only set after the resharing
	newGroup *key.Group
	// nodes that are created for running a first DKG
	nodes []*MockNode
	// new additional nodes that are created for running a resharing
	newNodes []*MockNode
	// nodes that actually ran the resharing phase - it's a combination of nodes
	// and new nodes. These are the one that should appear in the newGroup
	resharedNodes []*MockNode
}

// BatchNewDrand returns n drands, using TLS or not, with the given
// options. It returns the list of Drand structures, the group created,
// the folder where db, certificates, etc are stored. It is the folder
// to delete at the end of the test. As well, it returns a public grpc
// client that can reach any drand node.
// Deprecated: do not use
func BatchNewDrand(t *testing.T, n int, insecure bool, sch scheme.Scheme, opts ...ConfigOption) (
	drands []*Drand, group *key.Group, dir string, certPaths []string,
) {
	var privs []*key.Pair
	if insecure {
		privs, group = test.BatchIdentities(n, sch)
	} else {
		privs, group = test.BatchTLSIdentities(n, sch)
	}

	ports := test.Ports(n)
	drands = make([]*Drand, n)
	tmp := os.TempDir()

	dir, err := ioutil.TempDir(tmp, "drand")
	assert.NoError(t, err)

	certPaths = make([]string, n)
	keyPaths := make([]string, n)
	if !insecure {
		for i := 0; i < n; i++ {
			certPath := path.Join(dir, fmt.Sprintf("server-%d.crt", i))
			keyPath := path.Join(dir, fmt.Sprintf("server-%d.key", i))
			if httpscerts.Check(certPath, keyPath) != nil {
				h, _, err := gnet.SplitHostPort(privs[i].Public.Address())
				assert.NoError(t, err)

				t.Logf("generate keys for drand %d", i)
				err = httpscerts.Generate(certPath, keyPath, h)
				assert.NoError(t, err)
			}
			certPaths[i] = certPath
			keyPaths[i] = keyPath
		}
	}

	for i := 0; i < n; i++ {
		s := test.NewKeyStore()

		assert.NoError(t, s.SaveKeyPair(privs[i]))

		// give each one their own private folder
		dbFolder := path.Join(dir, fmt.Sprintf("db-%d", i))
		confOptions := []ConfigOption{WithDBFolder(dbFolder)}
		if !insecure {
			confOptions = append(confOptions,
				WithTLS(certPaths[i], keyPaths[i]),
				WithTrustedCerts(certPaths...))
		} else {
			confOptions = append(confOptions, WithInsecure())
		}

		confOptions = append(confOptions,
			WithControlPort(ports[i]),
			WithLogLevel(log.LogFatal, false))
		// add options in last so it overwrites the default
		confOptions = append(confOptions, opts...)

		t.Logf("Create drand %d", i)

		drands[i], err = NewDrand(s, NewConfig(confOptions...))
		assert.NoError(t, err)
	}

	return drands, group, dir, certPaths
}

// CloseAllDrands closes all drands
func CloseAllDrands(drands []*Drand) {
	for i := 0; i < len(drands); i++ {
		drands[i].Stop(context.Background())
	}
	for i := 0; i < len(drands); i++ {
		drands[i].WaitExit()
	}
}

// Deprecated: do not use sleeps in your tests
func getSleepDuration() time.Duration {
	if os.Getenv("CIRCLE_CI") != "" {
		fmt.Println("--- Sleeping on CIRCLECI")
		return time.Duration(600) * time.Millisecond
	}
	return time.Duration(500) * time.Millisecond
}

// NewDrandTest creates a drand test scenario with initial n nodes and ready to
// run a DKG for the given threshold that will then launch the beacon with the
// specified period
func NewDrandTestScenario(t *testing.T, n, thr int, period time.Duration, sch scheme.Scheme) *DrandTestScenario {
	dt := new(DrandTestScenario)

	drands, _, dir, certPaths := BatchNewDrand(
		t, n, false, sch, WithCallOption(grpc.WaitForReady(true)),
	)

	dt.t = t
	dt.dir = dir
	dt.certPaths = certPaths
	dt.groupPath = path.Join(dt.dir, "group.toml")
	dt.n = n
	dt.scheme = sch
	dt.thr = thr
	dt.period = period
	dt.clock = clock.NewFakeClock()
	dt.nodes = make([]*MockNode, 0, n)

	for i, drandInstance := range drands {
		node := newNode(dt.clock.Now(), dt.certPaths[i], drandInstance)
		dt.nodes = append(dt.nodes, node)
	}

	return dt
}

// Ids returns the list of the first n ids given the newGroup parameter (either
// in the original group or the reshared one)
// Deprecated: Rename this to addresses to align naming
func (d *DrandTestScenario) Ids(n int, newGroup bool) []string {
	nodes := d.nodes
	if newGroup {
		nodes = d.resharedNodes
	}

	var addresses []string
	for _, node := range nodes[:n] {
		addresses = append(addresses, node.addr)
	}

	return addresses
}

// RunDKG runs the DKG with the initial nodes created during NewDrandTest
func (d *DrandTestScenario) RunDKG() *key.Group {
	// common secret between participants
	secret := "thisisdkg"

	leaderNode := d.nodes[0]
	controlClient, err := net.NewControlClient(leaderNode.drand.opts.controlPort)
	require.NoError(d.t, err)

	d.t.Log("[RunDKG] Start")

	// the leaderNode will return the group over this channel
	var wg sync.WaitGroup
	wg.Add(d.n)

	// first run the leader and then run the other nodes
	go func() {
		d.t.Log("[RunDKG] Leader init")

		// TODO: Control Client needs every single parameter, not a protobuf type. This means that it will be difficult to extend
		groupPacket, err := controlClient.InitDKGLeader(
			d.n, d.thr, d.period, d.catchupPeriod, testDkgTimeout, nil, secret, testBeaconOffset, d.scheme.ID, "test_beacon")
		require.NoError(d.t, err)

		d.t.Log("[RunDKG] Leader obtain group")
		group, err := key.GroupFromProto(groupPacket)
		require.NoError(d.t, err)

		d.t.Logf("[RunDKG] Leader    Finished. GroupHash %x", group.Hash())
		wg.Done()
	}()

	// make sure the leader is up and running to start the setup
	// TODO: replace this with a ping loop and refactor to make it reusable
	time.Sleep(1 * time.Second)

	// all other nodes will send their PK to the leader that will create the group
	for _, node := range d.nodes[1:] {
		go func(n *MockNode) {
			client, err := net.NewControlClient(n.drand.opts.controlPort)
			require.NoError(d.t, err)
			groupPacket, err := client.InitDKG(leaderNode.drand.priv.Public, nil, secret)
			require.NoError(d.t, err)
			group, err := key.GroupFromProto(groupPacket)
			require.NoError(d.t, err)

			d.t.Logf("[RunDKG] NonLeader Finished. GroupHash %x", group.Hash())
			wg.Done()
		}(node)
	}

	// wait for all to return
	wg.Wait()
	d.t.Logf("[RunDKG] Leader %s FINISHED", leaderNode.addr)

	// we check that we can fetch the group using control functionalities on the leaderNode node
	groupProto, err := controlClient.GroupFile()
	require.NoError(d.t, err)
	group, err := key.GroupFromProto(groupProto)
	require.NoError(d.t, err)

	// we check all nodes are included in the group
	for _, node := range d.nodes {
		require.NotNil(d.t, group.Find(node.drand.priv.Public))
	}

	// we check the group has the right threshold
	require.Len(d.t, group.PublicKey.Coefficients, d.thr)
	require.Equal(d.t, d.thr, group.Threshold)
	require.NoError(d.t, key.Save(d.groupPath, group, false))

	d.group = group
	d.t.Log("[RunDKG] READY!")
	return group
}

func (d *DrandTestScenario) Cleanup() {
	_ = os.RemoveAll(d.dir)
	_ = os.RemoveAll(d.newDir)
}

// GetBeacon returns the beacon of the given round for the specified drand id
func (d *DrandTestScenario) GetBeacon(id string, round int, newGroup bool) (*chain.Beacon, error) {
	nodes := d.nodes
	if newGroup {
		nodes = d.resharedNodes
	}
	for _, node := range nodes {
		if node.addr != id {
			continue
		}
		return node.drand.beacon.Store().Get(uint64(round))
	}
	return nil, errors.New("that should not happen")
}

// GetMockNode returns the node associated with this address, either in the new
// group or the current group.
func (d *DrandTestScenario) GetMockNode(nodeAddress string, newGroup bool) *MockNode {
	nodes := d.nodes
	if newGroup {
		nodes = d.resharedNodes
	}

	for _, node := range nodes {
		if node.addr == nodeAddress {
			return node
		}
	}

	panic("no nodes found at this nodeAddress")
}

// StopMockNode stops a node from the first group
func (d *DrandTestScenario) StopMockNode(nodeAddr string, newGroup bool) {
	node := d.GetMockNode(nodeAddr, newGroup)

	dr := node.drand
	dr.Stop(context.Background())
	d.t.Logf("[drand] stop %s", dr.priv.Public.Address())

	controlClient, err := net.NewControlClient(dr.opts.controlPort)
	require.NoError(d.t, err)

	var retryCount = 1
	var maxRetries = 5
	for range time.Tick(100 * time.Millisecond) {
		d.t.Logf("[drand] ping %s: %d/%d", dr.priv.Public.Address(), retryCount, maxRetries)
		if err := controlClient.Ping(); err != nil {
			break
		}
		retryCount++
		require.LessOrEqual(d.t, retryCount, maxRetries)
	}

	d.t.Logf("[drand] stopped %s", dr.priv.Public.Address())
}

// StartDrand fetches the drand given the id, in the respective group given the
// newGroup parameter and runs the beacon
func (d *DrandTestScenario) StartDrand(nodeAddress string, catchup, newGroup bool) {
	node := d.GetMockNode(nodeAddress, newGroup)
	dr := node.drand

	// we load it from scratch as if the operator restarted its node
	newDrand, err := LoadDrand(dr.store, dr.opts)
	require.NoError(d.t, err)
	node.drand = newDrand

	d.t.Logf("[drand] Start")
	newDrand.StartBeacon(catchup)
	d.t.Logf("[drand] Started")
}

func (d *DrandTestScenario) Now() time.Time {
	return d.clock.Now()
}

// SetMockClock sets the clock of all drands to the designated unix timestamp in
// seconds
func (d *DrandTestScenario) SetMockClock(t *testing.T, targetUnixTime int64) {
	now := d.Now().Unix()
	if now < targetUnixTime {
		d.AdvanceMockClock(t, time.Duration(targetUnixTime-now)*time.Second)
	} else {
		d.t.Logf("ALREADY PASSED")
	}

	t.Logf("Set genesis time: %d", d.Now().Unix())
}

// AdvanceMockClock advances the clock of all drand by the given duration
func (d *DrandTestScenario) AdvanceMockClock(t *testing.T, p time.Duration) {
	for _, node := range d.nodes {
		node.clock.Advance(p)
	}
	for _, node := range d.newNodes {
		node.clock.Advance(p)
	}
	d.clock.Advance(p)
}

// CheckBeaconLength looks if the beacon chain on the given addresses is of the
// expected length (actual round plus 1, as beacons go from 0 to n)
func (d *DrandTestScenario) CheckBeaconLength(t *testing.T, nodes []*MockNode, expectedLength int) {
	for _, node := range nodes {
		err := d.WaitUntilRound(t, node, uint64(expectedLength-1))
		require.NoError(t, err)
	}
}

// CheckPublicBeacon looks if we can get the latest beacon on this node
func (d *DrandTestScenario) CheckPublicBeacon(nodeAddress string, newGroup bool) *drand.PublicRandResponse {
	node := d.GetMockNode(nodeAddress, newGroup)
	dr := node.drand

	client := net.NewGrpcClientFromCertManager(dr.opts.certmanager, dr.opts.grpcOpts...)
	resp, err := client.PublicRand(context.TODO(), test.NewTLSPeer(dr.priv.Public.Addr), &drand.PublicRandRequest{})

	require.NoError(d.t, err)
	require.NotNil(d.t, resp)
	return resp
}

// SetupNewNodes creates new additional nodes that can participate during the resharing
func (d *DrandTestScenario) SetupNewNodes(t *testing.T, newNodes int) []*MockNode {
	newDrands, _, newDir, newCertPaths := BatchNewDrand(d.t, newNodes, false, d.scheme,
		WithCallOption(grpc.WaitForReady(false)), WithLogLevel(log.LogDebug, false))
	d.newCertPaths = newCertPaths
	d.newDir = newDir
	d.newNodes = make([]*MockNode, newNodes)
	// add certificates of new nodes to the old nodes
	for _, node := range d.nodes {
		inst := node.drand
		for _, cp := range newCertPaths {
			err := inst.opts.certmanager.Add(cp)
			require.NoError(t, err)
		}
	}
	// store new part. and add certificate path of current nodes to the new
	d.newNodes = make([]*MockNode, 0, newNodes)
	for i, inst := range newDrands {
		node := newNode(d.clock.Now(), newCertPaths[i], inst)
		d.newNodes = append(d.newNodes, node)
		for _, cp := range d.certPaths {
			err := inst.opts.certmanager.Add(cp)
			require.NoError(t, err)
		}
	}
	return d.newNodes
}

func (d *DrandTestScenario) WaitUntilRound(t *testing.T, node *MockNode, round uint64) error {
	counter := 0

	newClient, err := net.NewControlClient(node.drand.opts.controlPort)
	require.NoError(t, err)

	for {
		status, err := newClient.Status()
		require.NoError(t, err)

		if !status.ChainStore.IsEmpty && status.ChainStore.LastRound == round {
			t.Logf("node %s is on expected round (%d)", node.addr, status.ChainStore.LastRound)
			return nil
		}

		counter++
		if counter == 10 {
			return fmt.Errorf("timeout waiting node %s to reach %d round", node.addr, round)
		}

		t.Logf("node %s is on %d round, waiting some time to ask again...", node.addr, status.ChainStore.LastRound)
		time.Sleep(1000 * time.Millisecond)
	}
}

func (d *DrandTestScenario) WaitUntilChainIsServing(t *testing.T, node *MockNode) error {
	counter := 0

	newClient, err := net.NewControlClient(node.drand.opts.controlPort)
	require.NoError(t, err)

	for {
		status, err := newClient.Status()
		require.NoError(t, err)

		if status.Beacon.IsServing {
			t.Logf("node %s has its beacon chain running", node.addr)
			return nil
		}

		counter++
		if counter == 10 {
			return fmt.Errorf("timeout waiting node %s to run beacon chain", node.addr)
		}

		t.Logf("node %s has its beacon chain not running yet, waiting some time to ask again...", node.addr)
		time.Sleep(500 * time.Millisecond)
	}
}

func (d *DrandTestScenario) runNodeReshare(n *MockNode, errCh chan error, force bool) {
	var secret = "thisistheresharing"

	leader := d.nodes[0]

	// instruct to be ready for resharing
	client, err := net.NewControlClient(n.drand.opts.controlPort)
	require.NoError(d.t, err)

	d.t.Logf("[reshare:node] init reshare")
	_, err = client.InitReshare(leader.drand.priv.Public, secret, d.groupPath, force)
	if err != nil {
		d.t.Log("[reshare:node] error in NON LEADER: ", err)
		errCh <- err
		return
	}

	d.t.Logf("[reshare]  non-leader drand %s DONE - %s", n.drand.priv.Public.Address(), n.drand.priv.Public.Key)
}

func (d *DrandTestScenario) runLeaderReshare(timeout time.Duration, errCh chan error, groupReceivedCh chan *key.Group) {
	var secret = "thisistheresharing"
	leader := d.nodes[0]

	oldNode := d.group.Find(leader.drand.priv.Public)
	if oldNode == nil {
		panic("[reshare:leader] leader not found in old group")
	}

	// old root: oldNode.Index leader: leader.addr
	client, err := net.NewControlClient(leader.drand.opts.controlPort)
	require.NoError(d.t, err)

	// Start reshare
	d.t.Logf("[reshare:leader] init reshare")
	finalGroup, err := client.InitReshareLeader(d.newN, d.newThr, timeout, 0, secret, "", testBeaconOffset)
	if err != nil {
		d.t.Log("[reshare:leader] error: ", err)
		errCh <- err
	}

	d.t.Logf("[reshare:leader] retrieve group")
	fg, err := key.GroupFromProto(finalGroup)
	if err != nil {
		errCh <- err
	}

	groupReceivedCh <- fg
}

// nolint:gocyclo
// RunReshare runs the resharing procedure with only "oldRun" current nodes
// running, and "newRun" new nodes running (the ones created via SetupNewNodes).
// It sets the given threshold to the group.
// It stops the nodes excluded first.
func (d *DrandTestScenario) RunReshare(t *testing.T, stateCh chan int,
	oldRun, newRun, newThr int,
	timeout time.Duration, force, onlyLeader bool, ignoreErr bool) (*key.Group, error) {
	if ignoreErr {
		d.t.Log("[reshare] WARNING IGNORING ERRORS!!!")
	}

	d.Lock()
	if stateCh != nil {
		stateCh <- ReshareLock
	}

	d.t.Logf("[reshare] LOCK")
	d.t.Logf("[reshare] old: %d/%d | new: %d/%d", oldRun, len(d.nodes), newRun, len(d.newNodes))

	// stop the excluded nodes
	for i, node := range d.nodes[oldRun:] {
		d.t.Logf("[reshare] stop old %d | %s", i, node.addr)
		d.StopMockNode(node.addr, false)
	}

	if len(d.newNodes) > 0 {
		for i, node := range d.newNodes[newRun:] {
			d.t.Logf("[reshare] stop new %d | %s", i, node.addr)
			d.StopMockNode(node.addr, true)
		}
	}

	d.newN = oldRun + newRun
	d.newThr = newThr
	leader := d.nodes[0]

	// Create channels
	errCh := make(chan error, 1)
	leaderGroupReadyCh := make(chan *key.Group, 1)

	// first run the leader, then the other nodes will send their PK to the
	// leader and then the leader will answer back with the new group
	go d.runLeaderReshare(timeout, errCh, leaderGroupReadyCh)
	d.resharedNodes = append(d.resharedNodes, leader)

	// leave some time to make sure leader is listening
	// Note: Remove this. Ping until leader is ready. Use a ping + lambda + retry
	time.Sleep(1 * time.Second)

	// run the current nodes
	for _, node := range d.nodes[1:oldRun] {
		d.resharedNodes = append(d.resharedNodes, node)
		if !onlyLeader {
			d.t.Logf("[reshare] run node reshare %s", node.addr)
			go d.runNodeReshare(node, errCh, force)
		}
	}

	// run the new ones
	if len(d.newNodes) > 0 {
		for _, node := range d.newNodes[:newRun] {
			d.resharedNodes = append(d.resharedNodes, node)
			if !onlyLeader {
				d.t.Logf("[reshare] run node reshare %s (new)", node.addr)
				go d.runNodeReshare(node, errCh, force)
			}
		}
	}

	d.t.Logf("[reshare] unlock")
	d.Unlock()

	if stateCh != nil {
		stateCh <- ReshareUnlock
	}

	// wait for the return of the clients
	for {
		select {
		case finalGroup := <-leaderGroupReadyCh:
			t.Logf("[reshare] Received group!")
			d.newGroup = finalGroup
			require.NoError(d.t, key.Save(d.groupPath, d.newGroup, false))
			d.t.Logf("[reshare] Finish")
			return finalGroup, nil

		case err := <-errCh:
			d.t.Logf("[reshare] ERROR: %s", err)
			if !ignoreErr {
				d.t.Logf("[reshare] Finish")
				return nil, err
			}

		case <-time.After(500 * time.Millisecond):
			d.AdvanceMockClock(t, d.period)
			t.Logf("[reshare] Advance clock: %d", d.Now().Unix())
		}
	}
}

// DenyClient can abort request to other needs based on a peer list
type DenyClient struct {
	t *testing.T
	net.ProtocolClient
	deny []string
}

func (d *Drand) DenyBroadcastTo(t *testing.T, addresses ...string) {
	client := d.privGateway.ProtocolClient
	d.privGateway.ProtocolClient = &DenyClient{
		t:              t,
		ProtocolClient: client,
		deny:           addresses,
	}
}

func (d *DenyClient) BroadcastDKG(c context.Context, p net.Peer, in *drand.DKGPacket, opts ...net.CallOption) error {
	if !d.isAllowed(p) {
		d.t.Logf("[DKG] Deny communication %s\n", p.Address())
		return errors.New("dkg broadcast denied")
	}

	return d.ProtocolClient.BroadcastDKG(c, p, in)
}

func (d *DenyClient) isAllowed(p net.Peer) bool {
	for _, s := range d.deny {
		if p.Address() == s {
			return false
		}
	}
	return true
}

func unixGetLimit() (curr, max uint64, err error) {
	rlimit := unix.Rlimit{}
	err = unix.Getrlimit(unix.RLIMIT_NOFILE, &rlimit)
	return rlimit.Cur, rlimit.Max, err
}

func unixSetLimit(soft, max uint64) error {
	rlimit := unix.Rlimit{
		Cur: soft,
		Max: max,
	}
	return unix.Setrlimit(unix.RLIMIT_NOFILE, &rlimit)
}

// newNode creates a node struct from a drand and sets the clock according to the drand test clock.
func newNode(now time.Time, certPath string, dr *Drand) *MockNode {
	id := dr.priv.Public.Address()
	c := clock.NewFakeClockAt(now)

	// Note: not pure
	dr.opts.clock = c

	return &MockNode{
		certPath: certPath,
		addr:     id,
		drand:    dr,
		clock:    c,
	}
}
