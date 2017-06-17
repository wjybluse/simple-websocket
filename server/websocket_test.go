package server

import (
	"fmt"
	"log"
	"testing"
)

func TestByteConvert(t *testing.T) {
	f := byte(255)
	fmt.Printf("f is %d, shift is %d", f, byte(254>>1))
}

func TestStringLen(t *testing.T) {
	s := []byte("hahha")
	log.Printf("len is %d \n", len(s))
}

func TestMask(t *testing.T) {
	mask := []byte{0x7a, 0x3f, 0xfe, 0x1f}
	password := []byte("this is a test")
	masked := make([]byte, len(password))
	for i := 0; i < len(password); i++ {
		masked[i] = password[i] ^ mask[i%4]
	}
	log.Printf("masked value is %s \n", string(masked))
	//transform
	origin := make([]byte, len(password))
	for i := 0; i < len(password); i++ {
		origin[i] = masked[i] ^ mask[i%4]
	}
	log.Printf("the origin is %s \n", string(origin))
}
