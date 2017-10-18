package cliupdater

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/kardianos/osext"
)

// DefaultCheckInterval is the minimum time between checks to see if the program is updated.
const DefaultCheckInterval = 24 * time.Hour
const checkSuffix = ".check"
const downloadSuffix = ".download"
const backupSuffix = ".backup"

func nilLogf(message string, args ...interface{}) {
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

// Updater keeps a Go binary up to date with a version available via HTTP(S). This can be used
// to implement auto-updating command line tools.
type Updater struct {
	// URL where the binary can be downloaded.
	BaseURL string
	// absolute path to the binary to be updated; defaults to the currently executing binary
	Path string
	// interval between automatic update checks
	CheckInterval time.Duration
	// Function used to output logs if not nil.
	Logf func(message string, args ...interface{})
	// Call the new binary with these arguments to "apply" an update. If it fails, the binary
	// will not be replaced.
	ApplyArgs []string
}

// Metadata contains information about the source binary and the binary on disk.
type Metadata struct {
	// The time the source binary was updated.
	Updated time.Time
	// The difference between the update time of the source binary and the binary on disk.
	Diff time.Duration
}

// Outdated returns true if the local binary is out of date.
func (u Metadata) Outdated() bool {
	return u.Diff > 0
}

// DaysOld returns the number of days that the local binary is out of date.
func (u Metadata) DaysOld() int {
	return int(u.Diff.Hours()/24 + 0.5)
}

// Returns an error if the fields are not set correctly
func (u *Updater) checkValidity() error {
	if u.BaseURL == "" {
		return errors.New("Updater: BaseURL is required")
	}
	if u.Logf == nil {
		u.Logf = nilLogf
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

// MaybeCheckForUpdate checks for an update if it has been long enough since the last check. It
// returns the metadata or an error if it executes a check. It returns a zero Metadata value if
// it does not check for an update.
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

// Update downloads the most recent version and replaces the current version.
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

	if len(u.ApplyArgs) != 0 {
		u.Logf("executing new binary with apply flags: %s", strings.Join(u.ApplyArgs, " "))
		cmd := exec.Command(updatePath, u.ApplyArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err := cmd.Run()
		if err != nil {
			u.Logf("new binary failed to apply update")
			return fmt.Errorf("Update() failed to apply update: %s", err.Error())
		}
		u.Logf("update applied successfully")
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
