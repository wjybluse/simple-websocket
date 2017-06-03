package utils

import (
	"crypto/sha1"
	"encoding/base64"
)

//Sha1 ... sha1
func Sha1(content string) []byte {
	sha := sha1.New()
	sha.Write([]byte(content))
	return sha.Sum(nil)
}

//Base64Encoding ... encoding string
func Base64Encoding(content []byte) string {
	return base64.StdEncoding.EncodeToString(content)
}

//Base64Decode ... decode string
func Base64Decode(content string) string {
	ds, _ := base64.StdEncoding.DecodeString(content)
	return string(ds)
}
