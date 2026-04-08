package main

import "os"

func main() {
	loadConfig()
	router(os.Stdin, os.Stdout)
}
