package core

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/drand/drand/common"
	"github.com/drand/drand/http"

	"github.com/drand/drand/utils"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"

	"github.com/drand/drand/net"
)

type DrandDaemon struct {
	candidates      map[string]bool
	stores          map[string]*key.Store
	beaconProcesses map[string]*BeaconProcess

	privGateway *net.PrivateGateway
	pubGateway  *net.PublicGateway
	control     net.ControlListener

	opts *Config
	log  log.Logger

	// global state lock
	state  sync.Mutex
	exitCh chan bool

	// version indicates the base code variant
	version utils.Version
}

func NewDrandDaemon(c *Config) (*DrandDaemon, error) {
	logger := c.Logger()
	if !c.insecure && (c.certPath == "" || c.keyPath == "") {
		return nil, errors.New("config: need to set WithInsecure if no certificate and private key path given")
	}

	return &DrandDaemon{
		opts:    c,
		log:     logger,
		exitCh:  make(chan bool, 1),
		version: common.GetAppVersion(),
	}, nil
}

func (dd *DrandDaemon) Init() error {
	c := dd.opts

	// Set the private API address to the command-line flag, if given.
	// Otherwise, set it to the address associated with stored private key.
	privAddr := c.PrivateListenAddress("")
	pubAddr := c.PublicListenAddress("")

	// ctx is used to create the gateway below.
	// Gateway constructors (specifically, the generated gateway stubs that require it)
	// do not actually use it, so we are passing a background context to be safe.
	ctx := context.Background()

	var err error
	dd.log.Infow("", "network", "init", "insecure", c.insecure)

	if pubAddr != "" {
		handler, err := http.New(ctx, &drandProxy{dd}, c.Version(), dd.log.With("server", "http"))
		if err != nil {
			return err
		}
		if dd.pubGateway, err = net.NewRESTPublicGateway(ctx, pubAddr, c.certPath, c.keyPath, c.certmanager, handler, c.insecure); err != nil {
			return err
		}
	}

	dd.privGateway, err = net.NewGRPCPrivateGateway(ctx, privAddr, c.certPath, c.keyPath, c.certmanager, dd, c.insecure, c.grpcOpts...)
	if err != nil {
		return err
	}

	p := c.ControlPort()
	dd.control = net.NewTCPGrpcControlListener(dd, p)
	go dd.control.Start()

	dd.log.Infow("", "private_listen", privAddr, "control_port", c.ControlPort(), "public_listen", pubAddr, "folder", c.ConfigFolder())
	dd.privGateway.StartAll()
	if dd.pubGateway != nil {
		dd.pubGateway.StartAll()
	}

	return nil
}

func (dd *DrandDaemon) CreateNewBeaconProcess(beaconID string, store key.Store) (*BeaconProcess, error) {
	priv, err := store.LoadKeyPair()
	if err != nil {
		return nil, err
	}
	if err := priv.Public.ValidSignature(); err != nil {
		return nil, fmt.Errorf("INVALID SELF SIGNATURE", err, "action", "run `drand util self-sign`")
	}

	bp := &BeaconProcess{
		store:       store,
		log:         dd.log,
		priv:        priv,
		version:     dd.version,
		opts:        dd.opts,
		privGateway: dd.privGateway,
		pubGateway:  dd.pubGateway,
		control:     dd.control,
		exitCh:      make(chan bool, 1),
	}

	dd.state.Lock()
	dd.beaconProcesses[beaconID] = bp
	dd.state.Unlock()

	return bp, nil
}
