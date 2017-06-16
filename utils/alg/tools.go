package alg

import (
	"bytes"
	"compress/flate"
	"io"
	"math/rand"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

//Encode encode value
type Encode interface {
	Encoding(value []byte) ([]byte, error)
}

//Decode value
type Decode interface {
	Decoding(value []byte) ([]byte, error)
}

//Deflat alg
type Deflat struct{}

//Encoding encode value
func (d *Deflat) Encoding(value []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer, err := flate.NewWriter(&buffer, 3)
	if err != nil {
		return nil, err
	}
	writer.Write(value)
	writer.Close()
	result := buffer.Bytes()
	if len(result) < 5 {
		return append(result, 0x0, 0x0), nil
	}
	lastBytes := result[len(result)-4:]
	deflatBlock := []byte{0x00, 0x00, 0xff, 0xff}
	for idx := range deflatBlock {
		if lastBytes[idx] != deflatBlock[idx] {
			return append(result, 0x0, 0x0), nil
		}
	}
	return result[:len(result)-5], nil
}

//Decoding decode value
func (d *Deflat) Decoding(value []byte) ([]byte, error) {
	//if message use deflat alg ,append \x00\x00\xff\xff\x01\x00\x00\xff\xff
	reader := flate.NewReader(bytes.NewBuffer(append(value, []byte{0x00, 0x00, 0xff, 0xff}...)))
	var buffer bytes.Buffer
	io.Copy(&buffer, reader)
	return buffer.Bytes(), nil
}

//RandomMask random mask gen
func RandomMask() []byte {
	return []byte(randSeq(4))
}
