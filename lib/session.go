package korra

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

type ResultEncoder struct {
	Name        string
	encoder     *gob.Encoder
	encoderFile io.WriteCloser
}

// name should be the script path
func NewResultEncoder(name string) *ResultEncoder {
	encoderDir := path.Dir(name)
	encoderName := strings.Replace(path.Base(name), ".txt", ".bin", -1)
	encoderFullPath := path.Join(encoderDir, encoderName)
	if encoderFile, err := os.Create(encoderFullPath); err != nil {
		panic(fmt.Sprintf("Cannot create encoder for results [Path: %s] [session file: %s] => %s", encoderFullPath, name, err))
	} else {
		encoder := gob.NewEncoder(encoderFile)
		return &ResultEncoder{encoderName, encoder, encoderFile}
	}
}

func (e *ResultEncoder) AddResult(r *Result) error {
	return e.encoder.Encode(r)
}

func (e *ResultEncoder) Close() {
	e.encoderFile.Close()
}

type Session struct {
	Name      string
	Script    *SessionScript
	attacker  *Attacker
	logTarget io.Writer
	stopper   chan struct{}
	running   bool
}

func NewSession(name string, in io.Reader, scriptPath string, opts []func(*Attacker)) (*Session, error) {
	var (
		err    error
		script *SessionScript
	)
	if script, err = NewScript(in, scriptPath); err != nil {
		return nil, err
	}
	logName := strings.Replace(name, ".txt", ".log", -1)
	logFile, _ := File(path.Join(scriptPath, logName), true)
	return &Session{
		Name:      name,
		Script:    script,
		attacker:  NewAttacker(opts...),
		logTarget: logFile,
		stopper:   make(chan struct{}),
	}, nil
}

// log writes to the session-only log file
func (session *Session) log(msg string) {
	if session.logTarget != nil {
		session.logTarget.Write([]byte(msg + "\n"))
	}
}

func (session *Session) Progress() SessionProgress {
	return session.Script.Progress()
}

func (session *Session) Run(log chan string) {
	session.running = true
	session.Script.Current = 1
	enc := NewResultEncoder(session.Name)
	results := make(chan *Result)
	go session.process(results, log)
	for {
		select {
		case result := <-results:
			enc.AddResult(result)

		// wait for the next result (or timeout) then wrap up:
		case <-session.stopper:
			session.running = false
			logMessage := fmt.Sprintf("%s: All done or asked to stop, waiting for next result or 5 seconds...", session.Name)
			log <- logMessage
			session.log(logMessage)
			select {
			case result := <-results:
				enc.AddResult(result)
			case <-time.After(5 * time.Second):
			}
			enc.Close()
			completeMessage := fmt.Sprintf("%s: ...DONE", session.Name)
			log <- completeMessage
			session.log(completeMessage)
			return
		}
	}
}

func (session *Session) Stop() {
	if session.running {
		session.stopper <- struct{}{}
	}
}

func (session *Session) process(results chan *Result, log chan string) {
	for _, action := range session.Script.Actions {
		label := fmt.Sprintf("%s (%s)", session.Name, session.Script.ProgressLabel())
		target := action.Target
		if target.IsComment() {
			session.log(fmt.Sprintf("%s: %s", label, target.Comment))
			continue
		} else if target.IsPause() {
			session.log(fmt.Sprintf("%s: Sleeping (%d ms)...", label, target.PauseTime))
			select {
			case <-session.stopper:
				return
			case <-time.After(time.Duration(target.PauseTime) * time.Millisecond):
			}
			continue
		}

		targeter := func() (*Target, error) { return target, nil }

		// retry a request if we're supposed to poll
		requests := 1
		for {
			timestamp := time.Now()
			result := session.attacker.Hit(targeter, timestamp)
			session.log(fmt.Sprintf("%s: %d => %s %s, %d ms",
				label, result.Code, result.Method, result.URL, int64(result.Latency/time.Millisecond)))
			results <- result
			if target.Poller.ShouldRetry(requests, int(result.Code)) {
				pauseMillis := target.Poller.WaitBetweenPolls
				session.log(fmt.Sprintf("%s: Pausing for %d ms until retry...", label, pauseMillis))
				time.Sleep(time.Duration(pauseMillis) * time.Millisecond)
				requests = requests + 1
			} else {
				break
			}
		}
	}
	session.stopper <- struct{}{}
}

func retryable(code uint16) bool {
	return code == 502 || code == 503 || code == 504
}
