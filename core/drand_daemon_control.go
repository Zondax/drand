package core

import (
	"context"
	"fmt"

	"github.com/drand/drand/common/scheme"

	"github.com/drand/drand/common/constants"
	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
)

// Control services
func (dd *DrandDaemon) InitDKG(c context.Context, in *drand.InitDKGPacket) (*drand.GroupPacket, error) {
	beaconID := ""
	if beaconID = in.GetMetadata().GetBeaconID(); beaconID == "" {
		beaconID = constants.DefaultBeaconID
	}

	if _, isBeaconInProgess := dd.candidates[beaconID]; isBeaconInProgess {
		return nil, fmt.Errorf("beacon id is already running DKG")
	}

	if _, isBeaconRunning := dd.beaconProcesses[beaconID]; isBeaconRunning {
		return nil, fmt.Errorf("beacon id is already running a randomness process")
	}

	if _, isStoreLoaded := dd.stores[beaconID]; !isStoreLoaded {
		return nil, fmt.Errorf("beacon id has no key store created, please do it first!")
	}

	// FIXME fix loading process
	store, _ := dd.stores[beaconID]
	bp, err := dd.CreateNewBeaconProcess(beaconID, *store)
	if err != nil {
		return nil, fmt.Errorf("something went wrong try to initiate DKG")
	}

	dd.state.Lock()
	dd.candidates[beaconID] = true
	dd.state.Unlock()

	resp, err := bp.InitDKG(c, in)

	dd.state.Lock()

	dd.candidates[beaconID] = false
	if err == nil {
		dd.beaconProcesses[beaconID] = bp
	}

	dd.state.Unlock()

	return resp, err
}

// InitReshare receives information about the old and new group from which to
// operate the resharing protocol.
func (dd *DrandDaemon) InitReshare(ctx context.Context, in *drand.InitResharePacket) (*drand.GroupPacket, error) {
	beaconID := ""
	if beaconID = in.GetMetadata().GetBeaconID(); beaconID == "" {
		beaconID = constants.DefaultBeaconID
	}

	if _, isBeaconRunning := dd.beaconProcesses[beaconID]; isBeaconRunning {
		return nil, fmt.Errorf("beacon id is not running")
	}

	bp, _ := dd.beaconProcesses[beaconID]
	return bp.InitReshare(ctx, in)
}

// PingPong simply responds with an empty packet, proving that this drand node
// is up and alive.
func (dd *DrandDaemon) PingPong(ctx context.Context, in *drand.Ping) (*drand.Pong, error) {
	metadata := common.NewMetadata(dd.version.ToProto())
	return &drand.Pong{Metadata: metadata}, nil
}

// Status responds with the actual status of drand process
func (dd *DrandDaemon) Status(ctx context.Context, in *drand.StatusRequest) (*drand.StatusResponse, error) {
	beaconID := ""
	if beaconID = in.GetMetadata().GetBeaconID(); beaconID == "" {
		beaconID = constants.DefaultBeaconID
	}

	if _, isBeaconRunning := dd.beaconProcesses[beaconID]; isBeaconRunning {
		return nil, fmt.Errorf("beacon id is not running")
	}

	bp, _ := dd.beaconProcesses[beaconID]
	return bp.Status(ctx, in)
}

func (dd *DrandDaemon) ListSchemes(ctx context.Context, in *drand.ListSchemesRequest) (*drand.ListSchemesResponse, error) {
	metadata := common.NewMetadata(dd.version.ToProto())

	return &drand.ListSchemesResponse{Ids: scheme.ListSchemes(), Metadata: metadata}, nil
}

// Share is a functionality of Control Service defined in protobuf/control that requests the private share of the drand node running locally
func (dd *DrandDaemon) Share(ctx context.Context, in *drand.ShareRequest) (*drand.ShareResponse, error) {
	beaconID := ""
	if beaconID = in.GetMetadata().GetBeaconID(); beaconID == "" {
		beaconID = constants.DefaultBeaconID
	}

	if _, isBeaconRunning := dd.beaconProcesses[beaconID]; isBeaconRunning {
		return nil, fmt.Errorf("beacon id is not running")
	}

	bp, _ := dd.beaconProcesses[beaconID]
	return bp.Share(ctx, in)
}

// PublicKey is a functionality of Control Service defined in protobuf/control
// that requests the long term public key of the drand node running locally
func (dd *DrandDaemon) PublicKey(ctx context.Context, in *drand.PublicKeyRequest) (*drand.PublicKeyResponse, error) {
	beaconID := ""
	if beaconID = in.GetMetadata().GetBeaconID(); beaconID == "" {
		beaconID = constants.DefaultBeaconID
	}

	if _, isBeaconRunning := dd.beaconProcesses[beaconID]; isBeaconRunning {
		return nil, fmt.Errorf("beacon id is not running")
	}

	bp, _ := dd.beaconProcesses[beaconID]
	return bp.PublicKey(ctx, in)
}

// PrivateKey is a functionality of Control Service defined in protobuf/control
// that requests the long term private key of the drand node running locally
func (dd *DrandDaemon) PrivateKey(ctx context.Context, in *drand.PrivateKeyRequest) (*drand.PrivateKeyResponse, error) {
	beaconID := ""
	if beaconID = in.GetMetadata().GetBeaconID(); beaconID == "" {
		beaconID = constants.DefaultBeaconID
	}

	if _, isBeaconRunning := dd.beaconProcesses[beaconID]; isBeaconRunning {
		return nil, fmt.Errorf("beacon id is not running")
	}

	bp, _ := dd.beaconProcesses[beaconID]
	return bp.PrivateKey(ctx, in)
}

// GroupFile replies with the distributed key in the response
func (dd *DrandDaemon) GroupFile(ctx context.Context, in *drand.GroupRequest) (*drand.GroupPacket, error) {
	beaconID := ""
	if beaconID = in.GetMetadata().GetBeaconID(); beaconID == "" {
		beaconID = constants.DefaultBeaconID
	}

	if _, isBeaconRunning := dd.beaconProcesses[beaconID]; isBeaconRunning {
		return nil, fmt.Errorf("beacon id is not running")
	}

	bp, _ := dd.beaconProcesses[beaconID]
	return bp.GroupFile(ctx, in)
}

// Shutdown stops the node
func (dd *DrandDaemon) Shutdown(ctx context.Context, in *drand.ShutdownRequest) (*drand.ShutdownResponse, error) {
	beaconID := ""
	if beaconID = in.GetMetadata().GetBeaconID(); beaconID == "" {
		beaconID = constants.DefaultBeaconID
	}

	if _, isBeaconRunning := dd.beaconProcesses[beaconID]; isBeaconRunning {
		return nil, fmt.Errorf("beacon id is not running")
	}

	bp, _ := dd.beaconProcesses[beaconID]
	bp.StopBeacon()

	return nil, nil
}

// BackupDatabase triggers a backup of the primary database.
func (dd *DrandDaemon) BackupDatabase(ctx context.Context, in *drand.BackupDBRequest) (*drand.BackupDBResponse, error) {
	beaconID := ""
	if beaconID = in.GetMetadata().GetBeaconID(); beaconID == "" {
		beaconID = constants.DefaultBeaconID
	}

	if _, isBeaconRunning := dd.beaconProcesses[beaconID]; isBeaconRunning {
		return nil, fmt.Errorf("beacon id is not running")
	}

	bp, _ := dd.beaconProcesses[beaconID]
	return bp.BackupDatabase(ctx, in)
}

func (dd *DrandDaemon) StartFollowChain(in *drand.StartFollowRequest, stream drand.Control_StartFollowChainServer) error {
	beaconID := ""
	if beaconID = in.GetMetadata().GetBeaconID(); beaconID == "" {
		beaconID = constants.DefaultBeaconID
	}

	if _, isBeaconRunning := dd.beaconProcesses[beaconID]; isBeaconRunning {
		return fmt.Errorf("beacon id is not running")
	}

	bp, _ := dd.beaconProcesses[beaconID]
	return bp.StartFollowChain(in, stream)

}

///////////

// Stop simply stops all drand operations.
func (dd *DrandDaemon) Stop(ctx context.Context) {
	for _, bp := range dd.beaconProcesses {
		bp.StopBeacon()
	}

	dd.state.Lock()
	if dd.pubGateway != nil {
		dd.pubGateway.StopAll(ctx)
	}
	dd.privGateway.StopAll(ctx)
	dd.control.Stop()
	dd.state.Unlock()

	dd.exitCh <- true
}

// WaitExit returns a channel that signals when drand stops its operations
func (dd *DrandDaemon) WaitExit() chan bool {
	return dd.exitCh
}
