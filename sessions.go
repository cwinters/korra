package main

import (
	"bytes"
	"crypto/tls"
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

	korra "github.com/cwinters/korra/lib"
)

func sessionsCmd() command {
	fs := flag.NewFlagSet("korra sessions", flag.ExitOnError)
	opts := &sessionsOpts{
		headers: headers{http.Header{}},
		laddr:   localAddr{&korra.DefaultLocalAddr},
	}

	fs.StringVar(&opts.certf, "cert", "", "x509 Certificate file")
	fs.StringVar(&opts.sessiond, "dir", "", "Directory of sessions")
	fs.Var(&opts.headers, "header", "Request header")
	fs.BoolVar(&opts.keepalive, "keepalive", true, "Use persistent connections")
	fs.Var(&opts.laddr, "laddr", "Local IP address")
	fs.StringVar(&opts.logf, "log", "stdout", "Overall log")
	fs.BoolVar(&opts.pretend, "pretend", false, "Do everything but send traffic")
	fs.IntVar(&opts.redirects, "redirects", korra.DefaultRedirects, "Number of redirects to follow. -1 will not follow but marks as success")
	fs.IntVar(&opts.statusSec, "status", 30, "Interval to log overall status, in seconds")
	fs.DurationVar(&opts.timeout, "timeout", korra.DefaultTimeout, "Requests timeout")

	return command{fs, func(args []string) error {
		fs.Parse(args)
		return Sessions(opts)
	}}
}

var (
	errBadCert    = errors.New("bad certificate")
	errMissingDir = errors.New("directory must exist and have at least one .txt file")
	timeFormat    = "15:04:05.999999"
)

// sessionOpts aggregates the session function command options
type sessionsOpts struct {
	certf     string
	headers   headers
	keepalive bool
	laddr     localAddr
	logf      string
	pretend   bool
	redirects int
	sessiond  string
	statusSec int
	timeout   time.Duration
}

// sessions validates the arguments, reads in the session scripts and launches
// them, providing a channel for each so it can halt them at will; it also
// provides a logger to display overall progress (starting and stopping, plus
// errors) and also writes to the logger every 30 seconds with the progress.
func Sessions(opts *sessionsOpts) error {
	var (
		err      error
		sessions []*korra.Session
		tlsc     *tls.Config
	)
	log := os.Stdout
	logChan := make(chan string)
	go func(o chan string) {
		for {
			select {
			case msg := <-o:
				out := fmt.Sprintf("%s %s\n", time.Now().Format(timeFormat), msg)
				log.Write([]byte(out))
			}
		}
	}(logChan)

	if tlsc, err = setupTLS(opts.certf); err != nil {
		return err
	}
	clientOptions := []func(*korra.Attacker){
		korra.Redirects(opts.redirects),
		korra.Timeout(opts.timeout),
		korra.LocalAddr(*opts.laddr.IPAddr),
		korra.TLSConfig(tlsc),
		korra.KeepAlive(opts.keepalive),
	}

	startTime := time.Now()

	sessionFiles := korra.GlobInputs(fmt.Sprintf("%s/*.txt", opts.sessiond))
	if sessions, err = readSessions(opts, sessionFiles, clientOptions, logChan); err != nil {
		return err
	}

	var wg sync.WaitGroup
	for _, aSession := range sessions {
		wg.Add(1)
		go func(session *korra.Session) {
			defer wg.Done()
			session.Run(logChan)
		}(aSession)
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
				session.Stop() // wait for each session to finish up?
			}
			return nil
		case <-time.After(time.Duration(opts.statusSec) * time.Second):
			actionCount, actionsDone, sessionsDone := 0, 0, 0
			for _, session := range sessions {
				progress := session.Progress()
				actionCount += progress.Actions
				actionsDone += progress.Current
				if progress.Complete {
					sessionsDone += 1
				}
			}
			sessionCount := len(sessions)
			logChan <- fmt.Sprintf("Elapsed %s: %d/%d actions complete (%.2f%%); %d/%d sessions complete (%.2f%%)",
				time.Since(startTime),
				actionsDone, actionCount, (float32(actionsDone)/float32(actionCount))*100,
				sessionsDone, sessionCount, (float32(sessionsDone)/float32(sessionCount))*100)
		}
	}
	return nil
}

func readSessions(opts *sessionsOpts, sessionFiles []string, clientOptions []func(*korra.Attacker), log chan string) ([]*korra.Session, error) {
	var err error
	sessions := make([]*korra.Session, len(sessionFiles))
	if len(sessionFiles) == 0 {
		return sessions, errMissingDir
	}
	for idx, sessionFile := range sessionFiles {
		if sessions[idx], err = korra.NewSession(sessionFile, clientOptions, log); err != nil {
			return sessions, fmt.Errorf("Error creating session script %s: %s", sessionFile, err)
		}
		sessions[idx].Pretend = opts.pretend
	}
	return sessions, nil
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

func setupTLS(filename string) (*tls.Config, error) {
	tlsc := *korra.DefaultTLSConfig
	if filename != "" {
		certf, err := korra.File(filename, false)
		if err != nil {
			return nil, fmt.Errorf("error opening %s: %s", filename, err)
		}
		var cert []byte
		if cert, err = ioutil.ReadAll(certf); err != nil {
			return nil, fmt.Errorf("error reading %s: %s", filename, err)
		}
		if tlsc.RootCAs, err = certPool(cert); err != nil {
			return nil, err
		}
	}
	return &tlsc, nil
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
