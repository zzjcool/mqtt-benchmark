package mqtt

import (
	"fmt"
	"testing"
)

func myIter(yield func(int) bool) {
	for i := 1; i < 10; i++ {
		if !yield(i) {
			fmt.Println("done")
			return
		}
		fmt.Println("continue")
	}
}

func TestXxx(t *testing.T) {
	fmt.Println("test")

	for i := range myIter {

		fmt.Println(i)
		if i > 5 {
			break
		}
	}
}
