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

func (dd *DrandDaemon) translateChainHashToBeaconId(metadata *protoCommon.Metadata) (string, error) {
	chainHash := ""
	chainHashSlice := metadata.GetChainHash()
	if chainHashSlice == nil || len(chainHashSlice) == 0 {
		chainHash = common.DefaultBeaconID
	} else {
		chainHash = fmt.Sprintf("%x", chainHashSlice)
	}

	dd.state.Lock()
	beaconID, isChainHashPresent := dd.chainHashes[chainHash]
	dd.state.Unlock()

	if !isChainHashPresent {
		return "", fmt.Errorf("beacon with chain hash [%s] is not running", chainHash)
	}

	return beaconID, nil

}
