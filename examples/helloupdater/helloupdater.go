package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/evanj/cliupdater"
)

func main() {
	fmt.Println("hello updater main()")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	update := flag.Bool("update", false, "Execute the self-update")
	nocheck := flag.Bool("nocheck", false, "Do not check for updates")
	flag.Parse()

	var logFunc func(string, ...interface{})
	if *verbose {
		logFunc = log.Printf
	}
	updater := &cliupdater.Updater{
		BaseURL: "https://storage.googleapis.com/cliupdater/helloupdater",
		Logf:    logFunc,
	}

	if *update {
		err := updater.Update()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing update: %s\n", err.Error())
			os.Exit(1)
		}
	} else if !*nocheck {
		metadata, err := updater.MaybeCheckForUpdate()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to check for update: %s\n", err.Error())
		} else if metadata.Outdated() {
			fmt.Fprintf(os.Stderr, "UPDATE: run with -update to update; your version is %d days old\n",
				metadata.DaysOld())
		}
	}
}
