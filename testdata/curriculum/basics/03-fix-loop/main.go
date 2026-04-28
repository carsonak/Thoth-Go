package main

import "fmt"

func main() {
	for i := 1; i <= 3; i++ { // BUG: should be i <= 5
		fmt.Println(i)
	}
}
