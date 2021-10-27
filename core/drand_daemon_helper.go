package core

import (
	"fmt"

	"github.com/drand/drand/common"
	protoCommon "github.com/drand/drand/protobuf/common"
)

func (dd *DrandDaemon) getBeaconProcessByID(metadata *protoCommon.Metadata) (*BeaconProcess, string, error) {
	beaconID := ""
	if beaconID = metadata.GetBeaconID(); beaconID == "" {
		beaconID = common.DefaultBeaconID
	}

	dd.state.Lock()
	bp, isBpCreated := dd.beaconProcesses[beaconID]
	dd.state.Unlock()

	if !isBpCreated {
		return nil, beaconID, fmt.Errorf("beacon id [%s] is not running", beaconID)
	}

	return bp, beaconID, nil
}

func (dd *DrandDaemon) getBeaconProcessByChainHash(metadata *protoCommon.Metadata) (*BeaconProcess, string, error) {
	chainHash := ""
	chainHashSlice := metadata.GetChainHash()
	if chainHashSlice == nil || len(chainHashSlice) == 0 {
		chainHash = common.DefaultBeaconID
	} else {
		chainHash = fmt.Sprintf("%x", chainHashSlice)
	}

	var bp *BeaconProcess
	isBpCreated := false

	dd.state.Lock()
	beaconID, isChainHashPresent := dd.chainHashes[chainHash]
	if isChainHashPresent {
		bp, isBpCreated = dd.beaconProcesses[beaconID]
	}
	dd.state.Unlock()

	if !isBpCreated {
		return nil, beaconID, fmt.Errorf("beacon id [%s] is not running", beaconID)
	}

	return bp, beaconID, nil

}
