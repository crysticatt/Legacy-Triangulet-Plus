package main

import (
	"io"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"
)

const (
	defaultDiscordID = "1569485882640044043"
	maxWorkers       = 16
	maxInFlight      = 32
)

// fix puts every setting into a sane range, so a hand-edited config.json can't break anything.
func (c *Config) fix() {
	if c.InFlight < 1 {
		c.InFlight = 1
	}
	if c.InFlight > maxInFlight {
		c.InFlight = maxInFlight
	}
	if c.Workers < 1 {
		c.Workers = 1
	}
	if c.Workers > maxWorkers {
		c.Workers = maxWorkers
	}
	switch c.Mode {
	case "fixed", "advance", "hunt", "least", "wish":
	default:
		c.Mode = "advance"
	}
	if c.HuntSpread < 1 {
		c.HuntSpread = 1
	}
	if c.HuntSpread > 9 {
		c.HuntSpread = 9
	}
	if c.CoinCost < 1 {
		c.CoinCost = 25
	}
	if c.StopAfterMystical < 0 {
		c.StopAfterMystical = 0
	}
	if c.AlertLowPacks < 0 {
		c.AlertLowPacks = 0
	}
	if c.DiscordID == "" {
		c.DiscordID = defaultDiscordID
	}
}

// One connection pool for every worker. Re-using already-open (HTTP/2) connections
// means a pull doesn't pay for a new TCP + TLS handshake each time.
var (
	transportOnce sync.Once
	transport     *http.Transport
)

// newTransport makes a connection pool of its own. Every worker uses one, so each worker
// opens its own connection(s) to the site instead of all of them sharing a single
// (HTTP/2) connection, which can become the bottleneck.
func newTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 128,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableCompression:  true, // replies are tiny; skipping gzip saves work
		WriteBufferSize:     16 << 10,
		ReadBufferSize:      16 << 10,
	}
}

// sharedTransport is one pool for everything that is not a worker.
func sharedTransport() *http.Transport {
	transportOnce.Do(func() { transport = newTransport() })
	return transport
}

// CoinPool is the account's coin balance as the opener believes it to be. Every worker
// takes the price of a capsule before it asks the site to open one, so together they
// stop exactly when the balance can't pay for another.
type CoinPool struct {
	mu       sync.Mutex
	v        int64 // -1 = unknown
	inflight int64
	leased   int64 // coins held in lanes' local credit (a part of inflight)
}

func newCoinPool() *CoinPool { return &CoinPool{v: -1} }

func (p *CoinPool) take(n int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.v < 0 {
		p.inflight += n
		return true // balance unknown: let the site decide
	}
	if p.v < n {
		return false
	}
	p.v -= n
	p.inflight += n
	return true
}

const leaseBatch = 8

// takeLease takes the price of up to leaseBatch capsules in one go, so a lane doesn't hit the
// shared lock on every request. Batches are only handed out while the balance is large; near the
// end every take is a single capsule, so stopping stays exact. n is how many capsules were taken.
func (p *CoinPool) takeLease(cost int64) (n int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.v < 0 {
		p.inflight += cost
		return 1 // balance unknown: let the site decide
	}
	n = 1
	if p.v >= cost*leaseBatch*64 {
		n = leaseBatch
	}
	if p.v < cost*n {
		return 0
	}
	p.v -= cost * n
	p.inflight += cost * n
	p.leased += cost * (n - 1)
	return n
}

// useLease turns one unit of a lane's credit into a request being paid for.
func (p *CoinPool) useLease(cost int64) {
	p.mu.Lock()
	p.leased -= cost
	p.mu.Unlock()
}

// returnLease gives back credit a lane no longer needs (it stopped or the request failed).
func (p *CoinPool) returnLease(cost, n int64) {
	if n <= 0 {
		return
	}
	p.mu.Lock()
	p.inflight -= cost * n
	p.leased -= cost * n
	if p.v >= 0 {
		p.v += cost * n
	}
	p.mu.Unlock()
}

// othersHold reports whether lanes still hold unused credit (so a failed take isn't final yet).
func (p *CoinPool) othersHold() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.leased > 0
}

// done settles one request: refund=true if no capsule was opened.
func (p *CoinPool) done(n int64, refund bool) {
	p.mu.Lock()
	p.inflight -= n
	if refund && p.v >= 0 {
		p.v += n
	}
	p.mu.Unlock()
}

// set takes the real balance from the site (minus what is still being paid for).
func (p *CoinPool) set(coins int64) {
	p.mu.Lock()
	p.v = coins - p.inflight
	if p.v < 0 {
		p.v = 0
	}
	p.mu.Unlock()
}

func (p *CoinPool) get() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.v
}

// sleepPrecise waits d. Very short waits spin (yielding to other goroutines) because
// the OS timer is far too coarse for them.
func sleepPrecise(d time.Duration) {
	if d <= 0 {
		return
	}
	if d >= 2*time.Millisecond {
		time.Sleep(d - time.Millisecond)
	}
	end := time.Now().Add(d)
	if d >= 2*time.Millisecond {
		end = time.Now().Add(time.Millisecond)
	}
	for time.Now().Before(end) {
		runtime.Gosched()
	}
}

// delayDur converts the delay setting (ms, may be tiny) to a Duration. Under 1 microsecond is "no wait".
func delayDur(ms float64) time.Duration {
	d := time.Duration(ms * float64(time.Millisecond))
	if d < time.Microsecond {
		return 0
	}
	return d
}

// slotWait sleeps until this lane's turn on the shared clock: with L lanes in total and
// a delay D, lane k fires at k*D/L inside every D-long period, so pulls interleave evenly.
func slotWait(t0 time.Time, lane, lanes int, d time.Duration) {
	if d <= 0 || lanes <= 1 {
		sleepPrecise(d)
		return
	}
	period := int64(d)
	off := period * int64(lane) / int64(lanes)
	el := int64(time.Since(t0))
	next := off
	if el >= off {
		next = (el-off)/period*period + period + off
	}
	sleepPrecise(time.Duration(next - el))
}

// Warm opens connections to the site before the first pull, so the first capsules don't pay for
// the TCP + TLS handshakes. It sends n cheap GETs at once and waits (at most a few seconds).
func (a *App) Warm(n int) {
	if n < 1 {
		n = 1
	}
	if n > 16 {
		n = 16
	}
	a.mu.Lock()
	base := a.cfg.BaseURL
	a.mu.Unlock()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequest("GET", base+"/", nil)
			if err != nil {
				return
			}
			resp, err := a.client.Do(req)
			if err != nil {
				return
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
	}
}
