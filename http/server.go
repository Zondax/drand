package http

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/drand/drand/common"

	"github.com/go-chi/chi"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	"github.com/drand/drand/log"
	"github.com/drand/drand/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	json "github.com/nikkolasg/hexjson"
)

const (
	watchConnectBackoff = 300 * time.Millisecond
	catchupExpiryFactor = 2
	roundNumBase        = 10
	roundNumSize        = 64
	chainHashParamKey   = "chainHash"
	roundParamKey       = "round"
)

var (
	// Timeout for how long to wait for the drand.PublicClient before timing out
	reqTimeout = 5 * time.Second
)

type DrandHandler struct {
	HandlerHTTP  http.Handler
	HandlerDrand *handler
}

type handler struct {
	timeout time.Duration

	context context.Context

	log log.Logger

	version string

	beacons map[string]*beaconHandler
	state   sync.Mutex
}

type beaconHandler struct {
	// NOTE: should only be accessed via getChainInfo
	chainInfo   *chain.Info
	chainInfoLk sync.RWMutex
	log         log.Logger

	// Client to handle beacon
	client client.Client

	// synchronization for blocking writes until randomness available.
	pendingLk   sync.RWMutex
	startOnce   sync.Once
	pending     []chan []byte
	context     context.Context
	latestRound uint64
	version     string
}

// New creates an HTTP handler for the public Drand API
func New(ctx context.Context, c client.Client, version string, logger log.Logger) (DrandHandler, error) {
	if logger == nil {
		logger = log.DefaultLogger()
	}
	handler := &handler{
		timeout: reqTimeout,
		log:     logger,
		context: ctx,
		version: version,
		beacons: make(map[string]*beaconHandler),
	}

	mux := chi.NewMux()

	mux.HandleFunc("/{"+chainHashParamKey+"}/public/latest", withCommonHeaders(version, handler.LatestRand))
	mux.HandleFunc("/{"+chainHashParamKey+"}/public/{"+roundParamKey+"}", withCommonHeaders(version, handler.PublicRand))
	mux.HandleFunc("/{"+chainHashParamKey+"}/info", withCommonHeaders(version, handler.ChainInfo))
	mux.HandleFunc("/{"+chainHashParamKey+"}/health", withCommonHeaders(version, handler.Health))

	mux.HandleFunc("/public/latest", withCommonHeaders(version, handler.LatestRand))
	mux.HandleFunc("/public/{"+roundParamKey+"}", withCommonHeaders(version, handler.PublicRand))
	mux.HandleFunc("/info", withCommonHeaders(version, handler.ChainInfo))
	mux.HandleFunc("/health", withCommonHeaders(version, handler.Health))

	instrumented := promhttp.InstrumentHandlerCounter(
		metrics.HTTPCallCounter,
		promhttp.InstrumentHandlerDuration(
			metrics.HTTPLatency,
			promhttp.InstrumentHandlerInFlight(
				metrics.HTTPInFlight,
				mux)))
	return DrandHandler{instrumented, handler}, nil
}

func (h *handler) CreateBeaconHandler(c client.Client, chainHash string) {
	h.state.Lock()
	defer h.state.Unlock()

	bh := &beaconHandler{
		context:     h.context,
		client:      c,
		latestRound: 0,
		pending:     nil,
		chainInfo:   nil,
		version:     h.version,
		log:         h.log,
	}

	h.beacons[chainHash] = bh
}

func withCommonHeaders(version string, h func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", version)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		h(w, r)
	}
}

func (h *handler) start(bh *beaconHandler) {
	bh.pendingLk.Lock()
	defer bh.pendingLk.Unlock()

	bh.pending = make([]chan []byte, 0)
	ready := make(chan bool)
	go h.Watch(bh, ready)

	<-ready
}

func (h *handler) Watch(bh *beaconHandler, ready chan bool) {
	for {
		select {
		case <-bh.context.Done():
			return
		default:
		}
		h.watchWithTimeout(bh, ready)
	}
}

func (h *handler) watchWithTimeout(bh *beaconHandler, ready chan bool) {
	watchCtx, cncl := context.WithCancel(bh.context)
	defer cncl()
	stream := bh.client.Watch(watchCtx)

	// signal that the watch is ready
	select {
	case ready <- true:
	default:
	}

	expectedRoundDelayBackoff := time.Minute
	bh.chainInfoLk.RLock()
	if bh.chainInfo != nil {
		expectedRoundDelayBackoff = bh.chainInfo.Period * 2
	}
	bh.chainInfoLk.RUnlock()
	for {
		var next client.Result
		var ok bool
		select {
		case <-bh.context.Done():
			return
		case next, ok = <-stream:
		case <-time.After(expectedRoundDelayBackoff):
			return
		}
		if !ok {
			h.log.Warnw("", "http_server", "random stream round failed")
			bh.pendingLk.Lock()
			bh.latestRound = 0
			bh.pendingLk.Unlock()
			// backoff on failures a bit to not fall into a tight loop.
			// TODO: tuning.
			time.Sleep(watchConnectBackoff)
			return
		}

		b, _ := json.Marshal(next)

		bh.pendingLk.Lock()
		if bh.latestRound+1 != next.Round() && bh.latestRound != 0 {
			// we missed a round, or similar. don't send bad data to peers.
			h.log.Warnw("", "http_server", "unexpected round for watch", "err", fmt.Sprintf("expected %d, saw %d", bh.latestRound+1, next.Round()))
			b = []byte{}
		}
		bh.latestRound = next.Round()
		pending := bh.pending
		bh.pending = make([]chan []byte, 0)

		for _, waiter := range pending {
			waiter <- b
		}
		bh.pendingLk.Unlock()
	}
}

func (h *handler) getChainInfo(ctx context.Context, chainHash []byte) *chain.Info {
	bh, err := h.getBeaconHandler(chainHash)
	if err != nil {
		return nil
	}

	bh.chainInfoLk.RLock()
	if bh.chainInfo != nil {
		info := bh.chainInfo
		bh.chainInfoLk.RUnlock()
		return info
	}
	bh.chainInfoLk.RUnlock()

	bh.chainInfoLk.Lock()
	defer bh.chainInfoLk.Unlock()

	if bh.chainInfo != nil {
		return bh.chainInfo
	}

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	info, err := bh.client.Info(ctx)
	if err != nil {
		h.log.Warnw("", "msg", "chain info fetch failed", "err", err)
		return nil
	}
	if info == nil {
		h.log.Warnw("", "msg", "chain info fetch didn't return group info")
		return nil
	}
	bh.chainInfo = info
	return info
}

func (h *handler) getRand(ctx context.Context, info *chain.Info, round uint64) ([]byte, error) {
	bh, err := h.getBeaconHandler(info.Hash())
	if err != nil {
		return nil, err
	}

	bh.startOnce.Do(func() {
		h.start(bh)
	})

	// First see if we should get on the synchronized 'wait for next release' bandwagon.
	block := false
	bh.pendingLk.RLock()
	block = (bh.latestRound+1 == round) && bh.latestRound != 0
	bh.pendingLk.RUnlock()
	// If so, prepare, and if we're still sync'd, add ourselves to the list of waiters.
	if block {
		ch := make(chan []byte, 1)
		defer close(ch)
		bh.pendingLk.Lock()
		block = (bh.latestRound+1 == round) && bh.latestRound != 0
		if block {
			bh.pending = append(bh.pending, ch)
		}
		bh.pendingLk.Unlock()

		// If that was successful, we can now block until we're notified.
		if block {
			select {
			case r := <-ch:
				return r, nil
			case <-ctx.Done():
				bh.pendingLk.Lock()
				defer bh.pendingLk.Unlock()
				for i, c := range bh.pending {
					if c == ch {
						bh.pending = append(bh.pending[:i], bh.pending[i+1:]...)
						break
					}
				}
				select {
				case <-ch:
				default:
				}
				return nil, ctx.Err()
			}
		}
	}

	// make sure we aren't going to ask for a round that doesn't exist yet.
	if time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, round), 0).After(time.Now()) {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	resp, err := bh.client.Get(ctx, round)

	if err != nil {
		return nil, err
	}

	return json.Marshal(resp)
}

func (h *handler) PublicRand(w http.ResponseWriter, r *http.Request) {
	// Get the round.
	round := readRound(r)
	roundN, err := strconv.ParseUint(round, roundNumBase, roundNumSize)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		h.log.Warnw("", "http_server", "failed to parse client round", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path))
		return
	}

	chainHashHex, err := readChainHash(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	info := h.getChainInfo(r.Context(), chainHashHex)
	roundExpectedTime := time.Now()
	if info == nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnw("", "http_server", "failed to get randomness", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	roundExpectedTime = time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, roundN), 0)

	if roundExpectedTime.After(time.Now().Add(info.Period)) {
		timeToExpected := int(time.Until(roundExpectedTime).Seconds())
		w.Header().Set("Cache-Control", fmt.Sprintf("public, must-revalidate, max-age=%d", timeToExpected))
		w.WriteHeader(http.StatusNotFound)
		h.log.Warnw("", "http_server", "request in the future", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path))
		return
	}

	data, err := h.getRand(r.Context(), info, roundN)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnw("", "http_server", "failed to get randomness", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}
	if data == nil {
		w.Header().Set("Cache-Control", "must-revalidate, no-cache, max-age=0")
		w.WriteHeader(http.StatusNotFound)
		h.log.Warnw("", "http_server", "request in the future", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path))
		return
	}

	// Headers per recommendation for static assets at
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	w.Header().Set("Expires", time.Now().Add(7*24*time.Hour).Format(http.TimeFormat))
	http.ServeContent(w, r, "rand.json", roundExpectedTime, bytes.NewReader(data))
}

func (h *handler) LatestRand(w http.ResponseWriter, r *http.Request) {
	chainHashHex, err := readChainHash(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	bh, err := h.getBeaconHandler(chainHashHex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	resp, err := bh.client.Get(ctx, 0)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnw("", "http_server", "failed to get randomness", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	data, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnw("", "http_server", "failed to marshal randomness", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	info := h.getChainInfo(r.Context(), chainHashHex)
	roundTime := time.Now()
	nextTime := time.Now()
	if info != nil {
		roundTime = time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, resp.Round()), 0)
		next := time.Unix(chain.TimeOfRound(info.Period, info.GenesisTime, resp.Round()+1), 0)
		if next.After(nextTime) {
			nextTime = next
		} else {
			nextTime = nextTime.Add(info.Period / catchupExpiryFactor)
		}
	}

	remaining := time.Until(nextTime)
	if remaining > 0 && remaining < info.Period {
		seconds := int(math.Ceil(remaining.Seconds()))
		w.Header().Set("Cache-Control", fmt.Sprintf("max-age:%d, public", seconds))
	} else {
		h.log.Warnw("", "http_server", "latest rand in the past",
			"client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "remaining", remaining)
	}

	w.Header().Set("Expires", nextTime.Format(http.TimeFormat))
	w.Header().Set("Last-Modified", roundTime.Format(http.TimeFormat))
	_, _ = w.Write(data)
}

func (h *handler) ChainInfo(w http.ResponseWriter, r *http.Request) {
	chainHashHex, err := readChainHash(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	info := h.getChainInfo(r.Context(), chainHashHex)
	if info == nil {
		w.WriteHeader(http.StatusNoContent)
		h.log.Warnw("", "http_server", "failed to serve group", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path))
		return
	}

	var chainBuff bytes.Buffer
	err = info.ToJSON(&chainBuff, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.log.Warnw("", "http_server", "failed to marshal group", "client", r.RemoteAddr, "req", url.PathEscape(r.URL.Path), "err", err)
		return
	}

	// Headers per recommendation for static assets at
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	w.Header().Set("Expires", time.Now().Add(7*24*time.Hour).Format(http.TimeFormat))
	http.ServeContent(w, r, "info.json", time.Unix(info.GenesisTime, 0), bytes.NewReader(chainBuff.Bytes()))
}

func (h *handler) Health(w http.ResponseWriter, r *http.Request) {
	chainHashHex, err := readChainHash(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	bh, err := h.getBeaconHandler(chainHashHex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	bh.startOnce.Do(func() {
		h.start(bh)
	})

	bh.pendingLk.RLock()
	lastSeen := bh.latestRound
	bh.pendingLk.RUnlock()

	info := h.getChainInfo(r.Context(), chainHashHex)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	resp := make(map[string]uint64)
	resp["current"] = lastSeen
	resp["expected"] = 0
	var b []byte

	if info == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		expected := chain.CurrentRound(time.Now().Unix(), info.Period, info.GenesisTime)
		resp["expected"] = expected
		if lastSeen == expected || lastSeen+1 == expected {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}

	b, _ = json.Marshal(resp)
	_, _ = w.Write(b)
}

func readChainHash(r *http.Request) ([]byte, error) {
	var err error
	chainHashHex := make([]byte, 0)

	chainHash := chi.URLParam(r, chainHashParamKey)
	if chainHash != "" {
		chainHashHex, err = hex.DecodeString(chainHash)
		if err != nil {
			return nil, fmt.Errorf("invalid chain hash")
		}
	}

	return chainHashHex, nil
}

func readRound(r *http.Request) string {
	round := chi.URLParam(r, roundParamKey)
	return round
}

func (h *handler) getBeaconHandler(chainHash []byte) (*beaconHandler, error) {
	chainHashStr := fmt.Sprintf("%x", chainHash)
	if chainHashStr == "" {
		chainHashStr = common.DefaultBeaconID
	}

	h.state.Lock()
	defer h.state.Unlock()

	bh, exists := h.beacons[chainHashStr]

	if !exists {
		return nil, fmt.Errorf("there's no BeaconHandler for beaconHash %s", chainHash)
	}

	return bh, nil
}
