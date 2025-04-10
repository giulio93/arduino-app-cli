package main

import (
	"fmt"
	"time"
)

func main() {
	for {
		fmt.Println("running...")
		time.Sleep(1 * time.Second)
	}
}
