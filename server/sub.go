package server

import (
	"encoding/binary"
	"log"
	"net"

	"strings"

	"fmt"

	"github.com/elians/websocket/utils/alg"
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
	fin         byte
	opcode      byte
	mask        byte
	payLoadLen  byte
	extLen      uint64
	maskingKey  []byte
	payloadData []byte
	rsv         byte
}

func (f *frame) toBytes() []byte {
	paylen := len(f.payloadData)
	data := []byte{(f.fin << 7) | (f.rsv << 4) | f.opcode}
	if paylen > 127 {
		var buffer []byte
		if paylen > (1<<16)+126 {
			f.payLoadLen = 127
			f.extLen = uint64(paylen - 127)
			binary.BigEndian.PutUint64(buffer, f.extLen)

		} else {
			f.payLoadLen = 126
			f.extLen = uint64(paylen - 126)
			binary.BigEndian.PutUint16(buffer, uint16(f.extLen))
		}
		data = append(data, (f.mask<<7)|f.payLoadLen)
		data = append(data, buffer...)

	} else {
		f.payLoadLen = byte(paylen)
		data = append(data, (f.mask<<7)|f.payLoadLen)
	}
	if f.mask == 1 {
		data = append(data, f.maskingKey[:4]...)
	}
	data = append(data, f.payloadData...)
	return data
}

func (f *frame) String() string {
	return fmt.Sprintf(`
		fin:%v,
		opcode:%v,
		mask: %v,
		paylen: %v,
		extlen: %d,
		maskkey: %v,
		payload: %v,
		rsv: %v
	`, f.fin, f.opcode, f.mask, f.payLoadLen, f.extLen, f.maskingKey, f.payloadData, f.rsv)
}

func newFrame(fin, opcode, mask, rsv byte, maskingKey, payload []byte) *frame {
	return &frame{
		fin:         fin,
		opcode:      opcode,
		mask:        mask,
		rsv:         rsv,
		maskingKey:  maskingKey,
		payloadData: payload,
	}
}

type defaultHandler struct {
}

func (d *defaultHandler) HandleTextMessage(msg string, reply Handler) error {
	log.Printf("handle text data %s \n", msg)
	return reply.Send([]byte("nihao"))
}

func (d *defaultHandler) HandleBinaryMessage(msg []byte, reply Handler) error {
	log.Printf("handle binary data %s \n", msg)
	return nil
}

type subprotocol struct {
	conn       net.Conn
	handler    MessageHandler
	frame      *frame
	extensions []string
}

//Handler ... simple interface for websocket impl
type Handler interface {
	//simple websocket method
	ping(data []byte) error
	pong(data []byte) error
	Send(msg []byte) error
	handleMessage() error
}

//NewHandler handler sub protocol
func NewHandler(conn net.Conn, handler MessageHandler, extensions []string) Handler {
	return &subprotocol{
		conn:       conn,
		handler:    handler,
		extensions: extensions,
	}
}

func (sb *subprotocol) enableDeflat() bool {
	for _, e := range sb.extensions {
		if strings.Contains(e, "deflat") {
			return true
		}
	}
	return false
}

func (sb *subprotocol) ping(data []byte) error {
	f := newFrame(0, OP_P, 0, 0, nil, []byte("are u ok?"))
	_, err := sb.conn.Write(f.toBytes())
	return err
}

func (sb *subprotocol) pong(data []byte) error {
	f := newFrame(0, OP_G, 0, 0, nil, []byte("i'm mibody"))
	_, err := sb.conn.Write(f.toBytes())
	return err
}

func (sb *subprotocol) Send(msg []byte) error {
	content := msg
	var err error
	if sb.enableDeflat() {
		deflat := alg.Deflat{}
		content, err = deflat.Encoding(msg)
		if err != nil {
			log.Printf("encoding data failed")
			return err
		}
	}
	f := newFrame(sb.frame.fin, sb.frame.opcode, sb.frame.mask, sb.frame.rsv, sb.frame.maskingKey, content)
	_, err = sb.conn.Write(f.toBytes())
	log.Printf("handle send data result is %s ,data is %v \n", err, f)
	return err
}

func (sb *subprotocol) createFrame() (*frame, error) {
	var buffer = make([]byte, 2)
	n, err := sb.conn.Read(buffer)
	if err != nil {
		//log.Printf("handle error %s \n", err)
		return nil, err
	}
	f := &frame{}
	if n < 2 {
		log.Printf("size is too short %s \n", err)
		return nil, fmt.Errorf("error size")
	}
	err = sb.validate(buffer[0], f)
	if err != nil {
		log.Printf("header is invalid %s \n", err)
		return nil, err
	}
	//mask and len
	sb.maskAndLen(buffer[1], f)
	if f.payLoadLen == 126 {
		var extBuffer = make([]byte, 2)
		n, err := sb.conn.Read(extBuffer)
		if n != 2 || err != nil {
			log.Printf("error when read extestion len %s \n", err)
			return nil, err
		}
		f.extLen, _ = binary.Uvarint(extBuffer)
	} else if f.payLoadLen == 127 {
		var extBuffer = make([]byte, 8)
		n, err := sb.conn.Read(extBuffer)
		if n != 8 || err != nil {
			log.Printf("error when read extestion len %s \n", err)
			return nil, fmt.Errorf("invalid size or error")
		}
		f.extLen, _ = binary.Uvarint(extBuffer)
	}
	if f.mask == 0 {
		//TODO
	} else {
		var maskBuffer = make([]byte, 4)
		n, err := sb.conn.Read(maskBuffer)
		if n != 4 || err != nil {
			log.Printf("err mask data %s \n", err)
			return nil, err
		}
		f.maskingKey = maskBuffer
	}
	//read data from connection
	totalLen := binary.BigEndian.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, f.payLoadLen}) + f.extLen
	f.payloadData = make([]byte, totalLen)
	n, err = sb.conn.Read(f.payloadData)
	if err != nil {
		log.Printf("handle error data %s \n", err)
		return nil, err
	}
	if uint64(n) != totalLen {
		log.Printf("read data error")
		return nil, err
	}
	sb.translate(f.payloadData[:totalLen], f.maskingKey)
	if sb.enableDeflat() {
		f.payloadData, _ = sb.decode(f.payloadData[:totalLen])
	}
	if err != nil {
		log.Printf("decode failed %s \n", err)
		sb.conn.Close()
		return nil, err
	}
	return f, nil
}

func (sb *subprotocol) handleMessage() error {
	for {
		f, err := sb.createFrame()
		if err != nil {
			continue
		}
		sb.frame = f
		switch f.opcode {
		case OP_P:
			sb.pong(nil)
			break
		case OP_G:
			sb.ping(nil)
			break
		case OP_T:
			sb.handler.HandleTextMessage(string(f.payloadData), sb)
			break
		case OP_B:
			sb.handler.HandleBinaryMessage(f.payloadData, sb)
			break
		case OP_S:
			sb.close()
			break
		default:
			break
		}
	}
}
func (sb *subprotocol) decode(payload []byte) ([]byte, error) {
	deflat := alg.Deflat{}
	return deflat.Decoding(payload)
}
func (sb *subprotocol) translate(payload, maskKey []byte) {
	for i := 0; i < len(payload); i++ {
		payload[i] = payload[i] ^ maskKey[i%4]
	}
}

func (sb *subprotocol) close() error {
	return sb.conn.Close()
}

func (sb *subprotocol) maskAndLen(segment byte, f *frame) {
	f.mask = (segment >> 7) & 1
	f.payLoadLen = (segment << 1) >> 1
}

//return opcode and error
func (sb *subprotocol) validate(frameHeader byte, f *frame) error {
	//validate fin
	//current only support simple impl
	f.fin = (frameHeader >> 7) & 1
	v := (frameHeader << 1) >> 5
	f.rsv = v
	f.opcode = (frameHeader << 4) >> 4
	return nil
}
