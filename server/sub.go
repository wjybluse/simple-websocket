package server

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
)

const (
	//this is op code
	OP_C byte = 0x0 //denotes a continuation frame
	OP_T byte = 0x1 //denotes a text frame
	OP_B byte = 0x2 //denotes a binary frame
	OP_S byte = 0x8 //denotes a connection close
	OP_P byte = 0x9 //ping
	OP_G byte = 0xA //pong
	//other for further
)

//MessageHandler ...
type MessageHandler interface {
	HandleTextMessage(msg string, reply Handler) error
	HandleBinaryMessage(msg []byte, reply Handler) error
}

//define header
//FIN,RSV1, RSV2, RSV3
/*
    0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
   +-+-+-+-+-------+-+-------------+-------------------------------+
   |F|R|R|R| opcode|M| Payload len |    Extended payload length    |
   |I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
   |N|V|V|V|       |S|             |   (if payload len==126/127)   |
   | |1|2|3|       |K|             |                               |
   +-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
   |     Extended payload length continued, if payload len == 127  |
   + - - - - - - - - - - - - - - - +-------------------------------+
   |                               |Masking-key, if MASK set to 1  |
   +-------------------------------+-------------------------------+
   | Masking-key (continued)       |          Payload Data         |
   +-------------------------------- - - - - - - - - - - - - - - - +
   :                     Payload Data continued ...                :
   + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
   |                     Payload Data continued ...                |
   +---------------------------------------------------------------+

*/
//this is simple subprotocol impl
//handle all payload
//simple Frame struct
type frame struct {
	opcode      int  //to [4]uint
	mask        uint //uint
	payLoadLen  int  //[7]uint
	extLen      uint64
	maskingKey  []byte
	payloadData []byte
}

type defaultHandler struct {
}

func (d *defaultHandler) HandleTextMessage(msg string, reply Handler) error {
	log.Printf("handle text data %s \n", msg)
	return nil
}

func (d *defaultHandler) HandleBinaryMessage(msg []byte, reply Handler) error {
	log.Printf("handle binary data %s \n", msg)
	return nil
}

func createPingMessage(data []byte) []byte {
	if data != nil {
		dl := len(data)
		d := []byte{OP_P, byte(dl)}
		d = append(d, data...)
		return d
	}
	return []byte{OP_P, 0x0}
}

func createPongMessage(data []byte) []byte {
	if data != nil {
		dl := len(data)
		d := []byte{OP_G, byte(dl)}
		d = append(d, data...)
		return d
	}
	return []byte{OP_G, 0x0}
}

type subprotocol struct {
	conn    net.Conn
	handler MessageHandler
}

//Handler ... simple interface for websocket impl
type Handler interface {
	//simple websocket method
	ping(data []byte) error
	pong(data []byte) error
	Send(msg []byte) error
	handleMessage() error
}

func newSubProtocol(conn net.Conn, handler MessageHandler) Handler {
	return &subprotocol{
		conn:    conn,
		handler: handler,
	}
}

func (sb *subprotocol) ping(data []byte) error {
	_, err := sb.conn.Write(createPingMessage(data))
	return err
}

func (sb *subprotocol) pong(data []byte) error {
	_, err := sb.conn.Write(createPongMessage(data))
	return err
}

func (sb *subprotocol) Send(msg []byte) error {
	return nil
}

func (sb *subprotocol) handleMessage() error {
	for {
		var buffer = make([]byte, 2)
		n, err := sb.conn.Read(buffer)
		if err != nil {
			log.Printf("handle error %s \n", err)
			continue
		}
		if n < 2 {
			log.Printf("size is too short %s \n", err)
			continue
		}
		opcode, err := sb.validate(buffer[0])
		if err != nil {
			log.Printf("header is invalid %s \n", err)
			continue
		}
		mask, pLen := sb.maskAndLen(buffer[1])
		var extLen uint64
		if pLen > 0 && pLen < 125 {
			//TODO
		} else if pLen == 126 {
			var extBuffer = make([]byte, 2)
			n, err := sb.conn.Read(extBuffer)
			if n != 2 || err != nil {
				log.Printf("error when read extestion len %s \n", err)
				continue
			}
			extLen, _ = binary.Uvarint(extBuffer)
		} else if pLen == 127 {
			var extBuffer = make([]byte, 8)
			n, err := sb.conn.Read(extBuffer)
			if n != 8 || err != nil {
				log.Printf("error when read extestion len %s \n", err)
				continue
			}
			extLen, _ = binary.Uvarint(extBuffer)
		}
		var maskKey []byte
		if mask == 0 {
			//TODO
		} else {
			var maskBuffer = make([]byte, 4)
			n, err := sb.conn.Read(maskBuffer)
			if n != 4 || err != nil {
				log.Printf("err mask data %s \n", err)
				continue
			}
			maskKey = maskBuffer
		}
		//read data from connection
		totalLen := binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, pLen}) + extLen
		var payload = make([]byte, totalLen)
		n, err = sb.conn.Read(payload)
		if err != nil {
			log.Printf("handle error data %s \n", err)
			continue
		}
		if uint64(n) != totalLen {
			log.Printf("read data error")
			continue
		}
		payload = sb.transformed(payload, maskKey)
		switch opcode {
		case OP_P:
			sb.pong(nil)
			break
		case OP_G:
			sb.ping(nil)
			break
		case OP_T:
			sb.handler.HandleTextMessage(string(payload), sb)
			break
		case OP_B:
			sb.handler.HandleBinaryMessage(payload, sb)
			break
		case OP_S:
			sb.close()
			break
		default:
			break
		}
	}
}

func (sb *subprotocol) transformed(payload, maskKey []byte) []byte {
	if maskKey == nil {
		return payload
	}
	var result = make([]byte, len(payload))
	for i := 0; i < len(payload); i++ {
		result[i] = payload[i] ^ maskKey[i%4]
	}
	return result[:len(payload)]
}

func (sb *subprotocol) close() error {
	return nil
}

func (sb *subprotocol) maskAndLen(segment byte) (byte, byte) {
	maskBit := segment >> 7
	mask := maskBit & 1
	length := (segment << 1)
	length = length >> 1
	return mask, length
}

//return opcode and error
func (sb *subprotocol) validate(frameHeader byte) (byte, error) {
	//validate fin
	//current only support simple impl
	var i byte
	realValue := frameHeader >> 4
	for i = 0; i < 4; i++ {
		r := realValue & i
		if r != 0 {
			return 0, fmt.Errorf("invalid header")
		}
	}
	//remove validate data
	opcode := (frameHeader << 4) >> 4
	return opcode, nil
}
