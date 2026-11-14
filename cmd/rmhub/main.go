// Command rmhub is a self-hosted sync server for reMarkable tablets.
package main

import "fmt"

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	fmt.Println("rmhub", version)
}
