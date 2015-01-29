package main

import (
	"bytes"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/cwinters/korra/lib"
)

func sessionCmd() command {
	fs := flag.NewFlagSet("korra sessions", flag.ExitOnError)
	opts := &sessionOpts{
		headers: headers{http.Header{}},
		laddr:   localAddr{&korra.DefaultLocalAddr},
	}

	fs.StringVar(&opts.certf, "cert", "", "x509 Certificate file")
	fs.Var(&opts.headers, "header", "Request header")
	fs.BoolVar(&opts.keepalive, "keepalive", true, "Use persistent connections")
	fs.Var(&opts.laddr, "laddr", "Local IP address")
	fs.StringVar(&opts.logf, "log", "stdout", "Overall log")
	fs.IntVar(&opts.redirects, "redirects", korra.DefaultRedirects, "Number of redirects to follow. -1 will not follow but marks as success")
	fs.StringVar(&opts.sessiond, "dir", "", "Directory of sessions")
	fs.DurationVar(&opts.timeout, "timeout", korra.DefaultTimeout, "Requests timeout")

	return command{fs, func(args []string) error {
		fs.Parse(args)
		return session(opts)
	}}
}

var (
	errBadCert    = errors.New("bad certificate")
	errMissingDir = errors.New("directory must exist and have at least one .txt file")
)

// sessionOpts aggregates the session function command options
type sessionOpts struct {
	certf     string
	headers   headers
	keepalive bool
	laddr     localAddr
	logf      string
	redirects int
	sessiond  string
	timeout   time.Duration
}

// session validates the attack arguments, sets up the
// required resources, launches the attack and writes the results
func session(opts *sessionOpts) error {
	tlsc := *korra.DefaultTLSConfig
	if opts.certf != "" {
		certf, err := file(opts.certf, false)
		if err != nil {
			return fmt.Errorf("error opening %s: %s", opts.certf, err)
		}
		var cert []byte
		if cert, err = ioutil.ReadAll(certf); err != nil {
			return fmt.Errorf("error reading %s: %s", opts.certf, err)
		}
		if tlsc.RootCAs, err = certPool(cert); err != nil {
			return err
		}
	}

	sessionOptions := []func(*korra.Attacker){
		korra.Redirects(opts.redirects),
		korra.Timeout(opts.timeout),
		korra.LocalAddr(*opts.laddr.IPAddr),
		korra.TLSConfig(&tlsc),
		korra.KeepAlive(opts.keepalive),
	}
	sessionFiles := globInputs(fmt.Sprintf("%s/*.txt", opts.sessiond))
	sessions := make([]*korra.Session, len(sessionFiles))
	for idx, sessionFile := range sessionFiles {
		reader, err := file(sessionFile, false)
		if err != nil {
			return fmt.Errorf("error reading session file %s: %s", sessionFile, err)
		}
		sessions[idx], err = korra.NewSession(sessionFile, reader, sessionOptions)
		if err != nil {
			return fmt.Errorf("Error creating session script %s: %s", sessionFile, err)
		}
	}

	var wg sync.WaitGroup

	for _, session := range sessions {
		wg.Add(1)
		go func(user *korra.Session) {
			defer wg.Done()
			session.Run()
		}(session)
	}

	// catch completion of all sessions, and from the OS
	var done = make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt)
	go func() {
		wg.Wait()
		done <- os.Interrupt
	}()

	for {
		select {
		case <-done:
			for _, session := range sessions {
				session.Stop() // wait for each user to finish up?
			}
			return nil
		}
	}

	return nil
}

// headers is the http.Header used in each target request
// it is defined here to implement the flag.Value interface
// in order to support multiple identical flags for request header
// specification
type headers struct{ http.Header }

func (h headers) String() string {
	buf := &bytes.Buffer{}
	if err := h.Write(buf); err != nil {
		return ""
	}
	return buf.String()
}

func (h headers) Set(value string) error {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("header '%s' has a wrong format", value)
	}
	key, val := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if key == "" || val == "" {
		return fmt.Errorf("header '%s' has a wrong format", value)
	}
	h.Add(key, val)
	return nil
}

// localAddr implements the Flag interface for parsing net.IPAddr
type localAddr struct{ *net.IPAddr }

func (ip *localAddr) Set(value string) (err error) {
	ip.IPAddr, err = net.ResolveIPAddr("ip", value)
	return
}

// certPool returns a new *x509.CertPool with the passed cert included.
// An error is returned if the cert is invalid.
func certPool(cert []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(cert) {
		return nil, errBadCert
	}
	return pool, nil
}
