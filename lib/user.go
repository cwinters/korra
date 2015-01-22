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

type User struct {
	Name     string
	Script   *SessionScript
	attacker *Attacker
	stopper  chan struct{}
	running  bool
}

func NewUser(name string, in io.Reader, opts []func(*Attacker)) (*User, error) {
	script, err := NewScript(in)
	if err != nil {
		return nil, err
	}
	return &User{
		Name:     name,
		Script:   script,
		attacker: NewAttacker(opts...),
		stopper:  make(chan struct{}),
	}, nil
}

type UserEncoder struct {
	Name        string
	encoderFile io.WriteCloser
	encoder     *gob.Encoder
}

func NewUserEncoder(name string) *UserEncoder {
	encoderDir := path.Dir(name)
	encoderName := strings.Replace(path.Base(name), ".txt", ".bin", -1)
	encoderFullPath := path.Join(encoderDir, encoderName)
	if encoderFile, err := os.Create(encoderFullPath); err != nil {
		panic(fmt.Sprintf("Cannot create encoder for results [Path: %s] [User file: %s] => %s", encoderFullPath, name, err))
	} else {
		encoder := gob.NewEncoder(encoderFile)
		return &UserEncoder{encoderName, encoderFile, encoder}
	}
}

func (e *UserEncoder) AddResult(r *Result) error {
	return e.encoder.Encode(r)
}

func (e *UserEncoder) Close() {
	e.encoderFile.Close()
}

func (user *User) Run() {
	user.running = true
	enc := NewUserEncoder(user.Name)
	results := make(chan *Result)
	go user.process(results)
	for {
		select {
		case result := <-results:
			enc.AddResult(result)

		// wait for the next result (or timeout) then wrap up:
		case <-user.stopper:
			user.running = false
			fmt.Fprintf(os.Stderr, "%s: All done or asked to stop, waiting for next result or 5 seconds...\n", user.Name)
			select {
			case result := <-results:
				enc.AddResult(result)
			case <-time.After(5 * time.Second):
			}
			enc.Close()
			fmt.Fprintf(os.Stderr, "%s: ...DONE\n", user.Name)
			return
		}
	}
}

func (user *User) Stop() {
	if user.running {
		user.stopper <- struct{}{}
	}
}

func (user *User) process(results chan *Result) {
	for _, action := range user.Script.Actions {
		label := fmt.Sprintf("%s (%s)", user.Name, user.Script.ProgressLabel())
		target := action.Target
		if target.IsComment() {
			fmt.Fprintf(os.Stderr, "%s: %s\n", label, target.Comment)
			continue
		} else if target.IsPause() {
			fmt.Fprintf(os.Stderr, "%s: Sleeping (%d ms)...\n", label, target.PauseTime)
			select {
			case <-user.stopper:
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
			result := user.attacker.hit(targeter, timestamp)
			fmt.Fprintf(os.Stderr, "%s: %d => %s %s, %d ms\n",
				label, result.Code, result.Method, result.URL, int64(result.Latency/time.Millisecond))
			results <- result
			if target.Poller.ShouldRetry(requests, result.Code) {
				pauseMillis := target.Poller.WaitBetweenPolls
				fmt.Fprintf(os.Stderr, "%s: Pausing for %d ms until retry...\n", label, pauseMillis)
				time.Sleep(time.Duration(pauseMillis) * time.Millisecond)
				requests = requests + 1
			} else {
				break
			}
		}
	}
	user.stopper <- struct{}{}
}

func retryable(code uint16) bool {
	return code == 502 || code == 503 || code == 504
}
