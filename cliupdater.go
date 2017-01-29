package cliupdater

import (
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/kardianos/osext"
)

const DefaultCheckInterval = 24 * time.Hour
const checkSuffix = ".check"
const downloadSuffix = ".download"
const backupSuffix = ".backup"

func NilLogf(message string, args ...interface{}) {
}

func goosToUname(goos string) string {
	return strings.Title(goos)
}

func unameOS() string {
	return goosToUname(runtime.GOOS)
}

var unameArches = map[string]string{
	"amd64": "x86_64",
}

func goarchToUname(goarch string) string {
	return unameArches[goarch]
}

func unameArch() string {
	return goarchToUname(runtime.GOARCH)
}

type Updater struct {
	// path to the binary
	BaseURL string
	// absolute path to the binary to be updated; defaults to the currently executing binary
	Path string
	// interval between automatic update checks
	CheckInterval time.Duration
	// called to output logs if not nil.
	Logf func(message string, args ...interface{})
}

type Metadata struct {
	Updated time.Time
	Diff    time.Duration
}

// Returns true if the local binary is out of date.
func (u Metadata) Outdated() bool {
	return u.Diff > 0
}

func (u Metadata) DaysOld() int {
	return int(u.Diff.Hours()/24 + 0.5)
}

// Returns an error if the fields are not set correctly
func (u *Updater) checkValidity() error {
	if u.BaseURL == "" {
		return errors.New("Updater: BaseURL is required")
	}
	if u.Logf == nil {
		u.Logf = NilLogf
	}
	if u.CheckInterval == time.Duration(0) {
		u.CheckInterval = DefaultCheckInterval
	}
	if u.CheckInterval <= 0 {
		return errors.New("Updater: CheckInterval must be > 0")
	}
	if u.Path == "" {
		var err error
		u.Path, err = osext.Executable()
		if err != nil {
			return err
		}
	}
	return nil
}

func (u *Updater) updateURL() string {
	return u.BaseURL + "-" + unameOS() + "-" + unameArch()
}

// Returns true if it is time to check for an update and a newer file is available.
func (u *Updater) MaybeCheckForUpdate() (Metadata, error) {
	err := u.checkValidity()
	if err != nil {
		return Metadata{}, err
	}

	dir, base := path.Split(u.Path)
	checkStampPath := dir + "." + base + checkSuffix
	u.Logf("reading timestamp from check file: %s ...", checkStampPath)
	var lastCheckTime time.Time
	fileinfo, err := os.Stat(checkStampPath)
	if err == nil {
		lastCheckTime = fileinfo.ModTime()
	} else if !os.IsNotExist(err) {
		// ignore "file does not exist" errors: means we've never checked for an update
		return Metadata{}, err
	}

	diff := time.Now().Sub(lastCheckTime)
	if diff < u.CheckInterval {
		u.Logf("not checking for update; last check %s; diff %s < interval %s",
			lastCheckTime, diff, u.CheckInterval)
		return Metadata{}, nil
	}

	// read the last modified time from the executable
	u.Logf("checking modified time of executable path: %s ...", u.Path)
	fileinfo, err = os.Stat(u.Path)
	if err != nil {
		return Metadata{}, err
	}

	// read the last modified time from HTTP HEAD
	url := u.updateURL()
	u.Logf("checking modified time of URL: %s ...", url)
	resp, err := http.Head(url)
	if err != nil {
		return Metadata{}, err
	}
	// discard any body (there should be none)
	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		return Metadata{}, err
	}
	err = resp.Body.Close()
	if err != nil {
		return Metadata{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Metadata{}, errors.New("status not 200 OK: " + resp.Status)
	}
	modifiedString := resp.Header.Get("Last-Modified")
	httpModified, err := time.Parse(time.RFC1123, modifiedString)
	if err != nil {
		return Metadata{}, err
	}

	// we completed a check: update our timestamp
	err = ioutil.WriteFile(checkStampPath, []byte{}, 0600)
	if err != nil {
		return Metadata{}, err
	}

	return Metadata{httpModified, httpModified.Sub(fileinfo.ModTime())}, nil
}

// Downloads the most recent version and attempts to replace the existing version.
func (u *Updater) Update() error {
	err := u.checkValidity()
	if err != nil {
		return err
	}

	// attempt to open the replacement temporary file
	dir, base := path.Split(u.Path)
	updatePath := dir + "." + base + downloadSuffix
	u.Logf("opening update file %s", updatePath)
	f, err := os.OpenFile(updatePath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0700)
	if err != nil {
		return err
	}
	defer f.Close()

	// start the download
	url := u.updateURL()
	u.Logf("downloading update from %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return errors.New("status not 200 OK: " + resp.Status)
	}
	// download the file
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	err = resp.Body.Close()
	if err != nil {
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}

	// backup the existing file and overwrite with the new
	// this is atomic so concurrently executing commands will work
	backupPath := dir + "." + base + backupSuffix
	u.Logf("linking existing exe to backup: %s", backupPath)
	err = os.Remove(backupPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	err = os.Link(u.Path, backupPath)
	if err != nil {
		return err
	}
	u.Logf("renaming downloaded file %s to final path: %s", updatePath, u.Path)
	return os.Rename(updatePath, u.Path)
}
