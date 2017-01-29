package cliupdater

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"time"

	"testing"
)

const fakeUpdate = "#!/bin/sh\necho hello\n"

type fixture struct {
	tempdir  string
	modified time.Time
	requests int
	server   *httptest.Server
	updater  Updater
}

func newFixture() (*fixture, error) {
	modified, err := time.Parse(time.RFC3339, "2017-01-01T12:34:56Z")
	if err != nil {
		return nil, err
	}

	tempdir, err := ioutil.TempDir("", "cliupdater_test")
	if err != nil {
		return nil, err
	}

	f := &fixture{tempdir, modified, 0, nil, Updater{}}
	f.server = httptest.NewServer(f)

	f.updater.BaseURL = f.server.URL + "/somebinary"
	f.updater.Logf = log.Printf
	f.updater.Path = f.tempdir + "/somebinary"
	err = ioutil.WriteFile(f.updater.Path, []byte{}, 0600)
	if err != nil {
		f.close()
		return nil, err
	}

	return f, nil
}

func (f *fixture) close() {
	f.server.Close()
	os.RemoveAll(f.tempdir)
}

func (f *fixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requests += 1
	if r.Method == "HEAD" {
		w.Header().Set("Last-Modified", f.modified.Format(time.RFC1123))
	} else if r.Method == "GET" {
		w.Write([]byte(fakeUpdate))
	} else {
		http.Error(w, "invalid method", http.StatusMethodNotAllowed)
	}
}

func TestMaybeCheckForUpdateBadArgs(t *testing.T) {
	updater := &Updater{}
	_, err := updater.MaybeCheckForUpdate()
	if err == nil {
		t.Error("no base url expected error")
	}
}

func TestMaybeCheckForUpdate(t *testing.T) {
	f, err := newFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()

	// without an existing "check" file, this should check for an update
	metadata, err := f.updater.MaybeCheckForUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Outdated() {
		t.Error("expected not outdated:", metadata)
	}
	if f.requests != 1 {
		t.Error("should have made an HTTP HEAD request", f.requests)
	}

	// checking again: too soon: should not check anything
	metadata, err = f.updater.MaybeCheckForUpdate()
	if err != nil {
		t.Fatal(err)
	}
	m := Metadata{}
	if metadata != m {
		t.Error("expected zero metadata:", metadata)
	}
	if f.requests != 1 {
		t.Error("should not have made any requests", f.requests)
	}

	// back date the check timestamp: should check and return true
	err = os.Chtimes(f.updater.Path, f.modified, f.modified)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Chtimes(f.tempdir+"/.somebinary"+checkSuffix, f.modified, f.modified)
	if err != nil {
		t.Fatal(err)
	}
	f.modified = time.Now()
	metadata, err = f.updater.MaybeCheckForUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if !metadata.Outdated() {
		t.Error("expected outdated:", metadata)
	}
	if f.requests != 2 {
		t.Error("should not have made any requests", f.requests)
	}
}

func TestUpdate(t *testing.T) {
	f, err := newFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()

	err = f.updater.Update()
	if err != nil {
		t.Fatal(err)
	}
	out, err := ioutil.ReadFile(f.updater.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != fakeUpdate {
		t.Error("unexpected contents of updated file:", string(out))
	}

	// run a second update: it should succeed
	err = f.updater.Update()
	if err != nil {
		t.Fatal(err)
	}
}

func TestGOOSToUname(t *testing.T) {
	tests := []struct {
		unameOS string
		goos    string
	}{
		{"Darwin", "darwin"},
		{"Linux", "linux"},
	}

	found := false
	for _, test := range tests {
		out := goosToUname(test.goos)
		if out != test.unameOS {
			t.Errorf("goosToUname(%s)=%s; expected %s", test.goos, out, test.unameOS)
		}
		if test.goos == runtime.GOOS {
			found = true
			if unameOS() != test.unameOS {
				t.Errorf("GOOS=%s; unameOS()=%s; expected %s", runtime.GOOS, unameOS(), test.unameOS)
			}
		}
	}
	if !found {
		t.Errorf("unexpected GOOS %s; add to this test", runtime.GOOS)
	}
}

func TestGOARCHToUname(t *testing.T) {
	tests := []struct {
		unameArch string
		goarch    string
	}{
		{"x86_64", "amd64"},
	}

	found := false
	for _, test := range tests {
		out := goarchToUname(test.goarch)
		if out != test.unameArch {
			t.Errorf("goarchToUname(%s)=%s; expected %s", test.goarch, out, test.unameArch)
		}
		if test.goarch == runtime.GOARCH {
			found = true
			if unameArch() != test.unameArch {
				t.Errorf("GOARCH=%s; unameArch()=%s; expected %s", runtime.GOARCH, unameArch(), test.unameArch)
			}
		}
	}
	if !found {
		t.Errorf("unexpected GOARCH %s; add to this test", runtime.GOARCH)
	}
}
