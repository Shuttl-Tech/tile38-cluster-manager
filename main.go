// +build !doc

package main

import (
	"log"
	"os"
)

func main() {
	err := App.Run(os.Args)
	if err != nil {
		log.Fatalf("manager failed with error. %s", err)
	}
}
