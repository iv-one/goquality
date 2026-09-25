package lib

import (
	"fmt"
	"testing"
)

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("bad")
	}
}

func Testhelper(t *testing.T) {} // not a test: lowercase after prefix

func BenchmarkAdd(b *testing.B) {}

func FuzzAdd(f *testing.F) {
	f.Add(1)
	f.Fuzz(func(t *testing.T, n int) {
		if Add(n, n) != 2*n {
			t.Fatal("bad")
		}
	})
}

func ExampleAdd() {
	fmt.Println(Add(1, 2))
	// Output: 3
}
