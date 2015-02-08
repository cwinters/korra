package korra

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Attacker is an attack executor which wraps an http.Client
type Attacker struct {
	dialer    *net.Dialer
	client    http.Client
	redirects int
}

var (
	// DefaultRedirects is the default number of times an Attacker follows
	// redirects.
	DefaultRedirects = 10
	// DefaultTimeout is the default amount of time an Attacker waits for a request
	// before it times out.
	DefaultTimeout = 30 * time.Second
	// DefaultLocalAddr is the default local IP address an Attacker uses.
	DefaultLocalAddr = net.IPAddr{IP: net.IPv4zero}
	// DefaultTLSConfig is the default tls.Config an Attacker uses.
	DefaultTLSConfig = &tls.Config{InsecureSkipVerify: true}

	// MarkRedirectsAsSuccess is the value when redirects are not followed but marked successful
	NoFollow = -1
)

// NewAttacker returns a new Attacker with default options which are overridden
// by the optionally provided opts.
func NewAttacker(opts ...func(*Attacker)) *Attacker {
	a := &Attacker{}
	a.dialer = &net.Dialer{
		LocalAddr: &net.TCPAddr{IP: DefaultLocalAddr.IP, Zone: DefaultLocalAddr.Zone},
		KeepAlive: 30 * time.Second,
		Timeout:   DefaultTimeout,
	}
	a.client = http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial:  a.dialer.Dial,
			ResponseHeaderTimeout: DefaultTimeout,
			TLSClientConfig:       DefaultTLSConfig,
			TLSHandshakeTimeout:   10 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// KeepAlive returns a functional option which toggles KeepAlive
// connections on the dialer and transport.
func KeepAlive(keepalive bool) func(*Attacker) {
	return func(a *Attacker) {
		tr := a.client.Transport.(*http.Transport)
		tr.DisableKeepAlives = !keepalive
		if !keepalive {
			a.dialer.KeepAlive = 0
			tr.Dial = a.dialer.Dial
		}
	}
}

// LocalAddr returns a functional option which sets the local address
// an Attacker will use with its requests.
func LocalAddr(addr net.IPAddr) func(*Attacker) {
	return func(a *Attacker) {
		tr := a.client.Transport.(*http.Transport)
		a.dialer.LocalAddr = &net.TCPAddr{IP: addr.IP, Zone: addr.Zone}
		tr.Dial = a.dialer.Dial
	}
}

// Redirects returns a functional option which sets the maximum
// number of redirects an Attacker will follow.
func Redirects(n int) func(*Attacker) {
	return func(a *Attacker) {
		a.redirects = n
		a.client.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
			if len(via) > n {
				return fmt.Errorf("stopped after %d redirects", n)
			}
			return nil
		}
	}
}

// Timeout returns a functional option which sets the maximum amount of time
// an Attacker will wait for a request to be responded to.
func Timeout(d time.Duration) func(*Attacker) {
	return func(a *Attacker) {
		tr := a.client.Transport.(*http.Transport)
		tr.ResponseHeaderTimeout = d
		a.dialer.Timeout = d
		tr.Dial = a.dialer.Dial
	}
}

// TLSConfig returns a functional option which sets the *tls.Config for a
// Attacker to use with its requests.
func TLSConfig(c *tls.Config) func(*Attacker) {
	return func(a *Attacker) {
		tr := a.client.Transport.(*http.Transport)
		tr.TLSClientConfig = c
	}
}

// Hit reads the next target from the targeter and sends the HTTP request with
// the headers and body from the Target, recording the bytes sent and received,
// the status code and error message.
func (a *Attacker) Hit(targeter Targeter, tm time.Time, requestCount int) *Result {
	var (
		err      error
		request  *http.Request
		response *http.Response
		result   = Result{Timestamp: tm, RequestCount: requestCount}
		tgt      *Target
	)

	defer func() {
		result.Latency = time.Since(tm)
		if err != nil {
			result.Error = err.Error()
		}
	}()

	if tgt, err = targeter(); err != nil {
		return &result
	}
	result.Method = tgt.Method
	result.PathFromURL(tgt.URL)

	if request, err = tgt.Request(); err != nil {
		return &result
	}

	if response, err = a.client.Do(request); err != nil {
		// ignore redirect errors when the user set --redirects=NoFollow
		if a.redirects == NoFollow && strings.Contains(err.Error(), "stopped after") {
			err = nil
		}
		return &result
	}
	response.Body.Close()

	if request.ContentLength != -1 {
		result.BytesOut = uint64(request.ContentLength)
	}

	if response.ContentLength != -1 {
		result.BytesIn = uint64(response.ContentLength)
	}

	if result.Code = uint16(response.StatusCode); result.HasErrorCode() {
		result.Error = response.Status
	}

	return &result
}
