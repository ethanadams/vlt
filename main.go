package main

import "github.com/ethanadams/vlt/cmd"

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cmd.Execute(version, commit, date)
}
