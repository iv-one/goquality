package main

import (
	"crypto/md5"
	"fmt"
	"os"

	"example.com/sample/lib"
)

func main() {
	os.Remove("a") // errcheck
	x := 1
	x = 2                     // ineffassign
	fmt.Printf("%d\n", "str") // vet printf
	fmt.Println(md5.Sum([]byte("a")), lib.Add(1, 2), x)
	os.Remove("b") //nolint:errcheck
	//nolint:errcheck // suppressed on the next line
	os.Remove("c")
	// this is definately misspelled
}
