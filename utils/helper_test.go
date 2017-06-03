package utils

import (
	"fmt"
	"testing"
)

func TestEncodeing(t *testing.T) {
	s := "dGhlIHNhbXBsZSBub25jZQ==258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	ss := Sha1(s)
	fmt.Println(Base64Encoding(ss))
}
