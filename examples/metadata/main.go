// Command metadata prints only the main executable's validated build metadata.
package main

import (
	"encoding/json"
	versioning "github.com/marcuoli/go-versioning"
	"log"
	"os"
)

func main() {
	info, err := versioning.CurrentBuild()
	if err != nil {
		log.Fatal(err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(info); err != nil {
		log.Fatal(err)
	}
}
