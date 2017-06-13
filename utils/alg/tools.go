package alg

import (
	"bytes"
	"compress/flate"
	"io"
)

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
	return buffer.Bytes(), nil
}

//Decoding decode value
func (d *Deflat) Decoding(value []byte) ([]byte, error) {
	reader := flate.NewReader(bytes.NewBuffer(append(value, []byte("\x00\x00\xff\xff\x01\x00\x00\xff\xff")...)))
	var buffer bytes.Buffer
	io.Copy(&buffer, reader)
	return buffer.Bytes(), nil
}
