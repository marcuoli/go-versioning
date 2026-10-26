// The app example reads metadata from its own binary without running Git.
package main

import (
	"fmt"
	"log"

	versioning "github.com/marcuoli/go-versioning"
)

var version, commit, buildTime string

func main() {
	info, err := versioning.Current(versioning.Info{Version: version, Commit: commit, BuildTime: buildTime})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Version: %s\nCommit: %s\nBuild time: %s\n", info.Version, info.Commit, info.BuildTime)
}
