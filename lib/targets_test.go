package korra

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestTargetRequest(t *testing.T) {
	t.Parallel()

	body, err := ioutil.ReadAll(io.LimitReader(rand.Reader, 1024*512))
	if err != nil {
		t.Fatal(err)
	}

	tgt := Target{
		Method: "GET",
		URL:    "http://:9999/",
		Body:   body,
		Header: http.Header{
			"X-Some-Header":       []string{"1"},
			"X-Some-Other-Header": []string{"2"},
			"X-Some-New-Header":   []string{"3"},
			"Host":                []string{"lolcathost"},
		},
	}
	req, _ := tgt.Request()

	reqBody, err := ioutil.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(tgt.Body, reqBody) {
		t.Fatalf("Target body wasn't copied correctly")
	}

	tgt.Header.Set("X-Stuff", "0")
	if req.Header.Get("X-Stuff") == "0" {
		t.Error("Each Target must have its own Header")
	}

	want, got := tgt.Header.Get("Host"), req.Header.Get("Host")
	if want != got {
		t.Fatalf("Target Header wasn't copied correctly. Want: %s, Got: %s", want, got)
	}
	if req.Host != want {
		t.Fatalf("Target Host wasnt copied correctly. Want: %s, Got: %s", want, req.Host)
	}
}

func TestTargetFromScanner(t *testing.T) {
	src := []string{
		"GET /foo/bar\nHeader:Value",
		"POST /foo/bar/baz\nHeader:Value\n@../CHANGELOG",
		"POST /buzzer\nHeader:Bees\nHeader-Two:Honey\n@../CHANGELOG",
		"HEAD /foos",
	}
	singleHeader := http.Header{}
	singleHeader.Add("Header", "Value")
	doubleHeader := http.Header{}
	doubleHeader.Add("Header", "Bees")
	doubleHeader.Add("Header-Two", "Honey")
	postBody, _ := ioutil.ReadFile("../CHANGELOG")

	var emptyBody []byte
	var emptyHeaders http.Header

	wants := []*Target{
		&Target{Method: "GET", URL: "/foo/bar", Header: singleHeader, Body: emptyBody},
		&Target{Method: "POST", URL: "/foo/bar/baz", Header: singleHeader, Body: postBody},
		&Target{Method: "POST", URL: "/buzzer", Header: doubleHeader, Body: postBody},
		&Target{Method: "HEAD", URL: "/foos", Header: http.Header{}, Body: emptyBody},
	}

	for idx, text := range src {
		want := wants[idx]
		scanner := peekingScanner{src: bufio.NewScanner(strings.NewReader(text))}
		got, err := TargetFromScanner(scanner, emptyBody, emptyHeaders, fmt.Sprintf("Item %d", idx))
		if err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(want, got) {
			t.Fatalf("want: %#v, got: %#v", want, got)
		}
	}
}

func TestNewEagerTargeter(t *testing.T) {
	t.Parallel()

	src := []byte("GET http://:6060/\nHEAD http://:6606/")
	read, err := NewEagerTargeter(bytes.NewReader(src), []byte("body"), nil)
	if err != nil {
		t.Fatalf("Couldn't parse valid source: %s", err)
	}
	for _, want := range []*Target{
		&Target{
			Method: "GET",
			URL:    "http://:6060/",
			Body:   []byte("body"),
			Header: http.Header{},
		},
		&Target{
			Method: "HEAD",
			URL:    "http://:6606/",
			Body:   []byte("body"),
			Header: http.Header{},
		},
	} {
		if got, err := read(); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(want, got) {
			t.Fatalf("want: %#v, got: %#v", want, got)
		}
	}
}

func TestNewLazyTargeter(t *testing.T) {
	for want, def := range map[error]string{
		errors.New("bad target"): "GET",
		errors.New("bad method"): "SET http://:6060",
		errors.New("bad URL"):    "GET foobar",
		errors.New("bad body"): `
			GET http://:6060
			@238hhqwjhd8hhw3r.txt`,
		errors.New("bad header"): `
		  GET http://:6060
			Authorization`,
		errors.New("bad header"): `
			GET http://:6060
			Authorization:`,
		errors.New("bad header"): `
			GET http://:6060
			: 1234`,
	} {
		src := bytes.NewBufferString(strings.TrimSpace(def))
		read := NewLazyTargeter(src, []byte{}, http.Header{})
		if _, got := read(); got == nil || !strings.HasPrefix(got.Error(), want.Error()) {
			t.Errorf("got: %s, want: %s\n%s", got, want, def)
		}
	}

	bodyf, err := ioutil.TempFile("", "korra-")
	if err != nil {
		t.Fatal(err)
	}
	defer bodyf.Close()
	defer os.Remove(bodyf.Name())
	bodyf.WriteString("Hello world!")

	targets := fmt.Sprint(`
		GET http://:6060/
		X-Header: 1
		X-Header: 2

		PUT https://:6060/123

		POST http://foobar.org/fnord
		Authorization: x12345
		@`, bodyf.Name(),
	)

	src := bytes.NewBufferString(strings.TrimSpace(targets))
	read := NewLazyTargeter(src, []byte{}, http.Header{"Content-Type": []string{"text/plain"}})
	for _, want := range []*Target{
		&Target{
			Method: "GET",
			URL:    "http://:6060/",
			Body:   []byte{},
			Header: http.Header{
				"X-Header":     []string{"1", "2"},
				"Content-Type": []string{"text/plain"},
			},
		},
		&Target{
			Method: "PUT",
			URL:    "https://:6060/123",
			Body:   []byte{},
			Header: http.Header{"Content-Type": []string{"text/plain"}},
		},
		&Target{
			Method: "POST",
			URL:    "http://foobar.org/fnord",
			Body:   []byte("Hello world!"),
			Header: http.Header{
				"Authorization": []string{"x12345"},
				"Content-Type":  []string{"text/plain"},
			},
		},
	} {
		if got, err := read(); err != nil {
			t.Fatal(err)
		} else if !reflect.DeepEqual(want, got) {
			t.Fatalf("want: %#v, got: %#v", want, got)
		}
	}
	if got, err := read(); err != ErrNoTargets {
		t.Fatalf("got: %v, want: %v", err, ErrNoTargets)
	} else if got != nil {
		t.Fatalf("got: %v, want: %v", got, nil)
	}
}
