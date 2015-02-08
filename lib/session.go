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
func NewResultEncoder(scriptPath string) *ResultEncoder {
	encoderDir := path.Dir(scriptPath)
	encoderName := strings.Replace(path.Base(scriptPath), ".txt", ".bin", -1)
	encoderFullPath := path.Join(encoderDir, encoderName)
	if encoderFile, err := os.Create(encoderFullPath); err != nil {
		panic(fmt.Sprintf("Cannot create encoder for results [Path: %s] [session file: %s] => %s", encoderFullPath, scriptPath, err))
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
	Name     string
	Path     string
	Pretend  bool
	Script   *SessionScript
	attacker *Attacker
	logChan  chan string
	results  chan *Result
	running  bool
	stopper  chan struct{}
}

func NewSession(scriptPath string, opts []func(*Attacker), logChan chan string) (*Session, error) {
	var (
		err    error
		script *SessionScript
	)

	if script, err = NewScript(scriptPath); err != nil {
		return nil, err
	}
	name := path.Base(scriptPath)
	session := &Session{
		Name:     name,
		Path:     scriptPath,
		Script:   script,
		attacker: NewAttacker(opts...),
		logChan:  logChan,
		results:  make(chan *Result),
		stopper:  make(chan struct{}),
	}
	session.log("CREATED")
	return session, nil
}

// log sends messages to the global log, prefixing it first with the session
// name and the current progress
func (session *Session) log(msg string) {
	if session.logChan != nil {
		session.logChan <- fmt.Sprintf("%s %s: %s", session.Name, session.Script.ProgressLabel(), msg)
	}
}

func (session *Session) Progress() SessionProgress {
	return session.Script.Progress()
}

func (session *Session) Run(log chan string) {
	session.running = true
	enc := NewResultEncoder(session.Path)
	go session.process(log)
	for {
		select {
		case result := <-session.results:
			enc.AddResult(result)

		// wait for the next result (or timeout) then wrap up:
		case <-session.stopper:
			session.running = false
			session.log("All done or asked to stop, waiting for next result or 5 seconds...")
			if !session.Pretend {
				select {
				case result := <-session.results:
					enc.AddResult(result)
				case <-time.After(5 * time.Second):
				}
			}
			enc.Close()
			session.log("DONE")
			return
		}
	}
}

func (session *Session) Stop() {
	if session.running {
		session.stopper <- struct{}{}
	}
}

func (session *Session) process(log chan string) {
	for session.Script.ActionsRemain() {
		action := session.Script.NextAction()
		target := action.Target
		if target.IsComment() {
			session.log(target.Comment)
		} else if target.IsPause() {
			session.pause(target.PauseTime)
		} else {
			session.doHttp(action)
		}
	}
	session.stopper <- struct{}{}
}

func (session *Session) pause(pauseMillis int) {
	if session.Pretend {
		session.log(fmt.Sprintf("Sleeping (pretend) (%d ms)...", pauseMillis))
		return
	}
	session.log(fmt.Sprintf("Sleeping (%d ms)...", pauseMillis))
	select {
	case <-session.stopper:
		return
	case <-time.After(time.Duration(pauseMillis) * time.Millisecond):
	}
}

func (session *Session) doHttp(action *SessionAction) {
	target := action.Target
	if session.Pretend {
		session.log(fmt.Sprintf("%d (pretend) => %s %s, %d ms",
			200, target.Method, target.URL, 0))
		return
	}
	targeter := func() (*Target, error) { return target, nil }

	// retry a request if we're supposed to poll
	requests := 1
	for {
		timestamp := time.Now()
		result := session.attacker.Hit(targeter, timestamp, requests)
		session.log(fmt.Sprintf("%d => %s %s, %d ms",
			result.Code, result.Method, result.Path, int64(result.Latency/time.Millisecond)))
		session.results <- result
		if target.Poller.ShouldRetry(requests, int(result.Code)) {
			pauseMillis := target.Poller.WaitBetweenPolls
			session.log(fmt.Sprintf("Attempt %d requires retry, %d ms pause until next poll", requests, pauseMillis))
			time.Sleep(time.Duration(pauseMillis) * time.Millisecond)
			requests += 1
		} else {
			break
		}
	}
}

func retryable(code uint16) bool {
	return code == 502 || code == 503 || code == 504
}
