# cliupdater: Download and keep a Go command-line tool up to date

This is a hack proof-of-concept designed similarly to gcloud's auto-update mechanism. The intention is:

* At most once a day, the command checks if an update is available.
* If there is an update, the tool displays some information about it, and provides a command that can be run to install the update.
* Executing the update command causes the new version to be downloaded and the existing version to be replaced.

This proof-of-concept does this in the simplest way I could think of:

* The binary creates a zero-length hidden "stamp" file with the name `.binary.check` to record the timestamp of the last update check.
* On startup, if this file is older than a day, it does an HTTP HEAD request to a download URL (customized for the OS and architecture). This request returns a `Last-Modified` header. The value of this header is compared to to the timestamp of the executable. If it is more recent, the tool can print a message.
* If the user invokes `Update`, it performs an update:
  - Downloads the file as `.binary.download`
  - Backs up the existing file as `.binary.backup`
  - Renames the download over the existing file

Ideally, the update should be atomic, so multiple concurrent updates or concurrently executing commands will not cause any problems.


## TODO

* Verify the downloaded file? E.g. check length or a signature?
* Display release notes or changes?
