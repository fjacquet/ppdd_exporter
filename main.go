// Command ppdd_exporter is a Prometheus exporter for Dell PowerProtect DD appliances.
package main

import "fmt"

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	fmt.Printf("ppdd_exporter %s\n", version)
}
