package server

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"time"

	"strings"

	"fmt"

	"github.com/elians/websocket/utils/alg"
)

const (
	opc byte = 0x0 //denotes a continuation frame
	opt byte = 0x1 //denotes a text frame
	opb byte = 0x2 //denotes a binary frame
	ops byte = 0x8 //denotes a connection close
	opp byte = 0x9 //ping
	opg byte = 0xA //pong
	//other for further

	payloadFixLen = 0x7f - 0x01
	payloadMaxLen = 0x7f

	extLenFix = 0x10
	extLenMax = 0x40
)

const (
	normlClose int16 = 1000 + iota
	goAway
	protocolErr
	acceptErr
	reserved
	reserved1
	abnormal
	typeErr
	outsideErr
	msgBigErr
	extensionNotSupport
	requestErr
	tlsErr
)

var (
	reasons = map[int16]string{
		normlClose:          "indicates a normal closure",
		goAway:              "endpoint is going away",
		protocolErr:         "protocol error",
		acceptErr:           "received a type of data it cannot accept",
		reserved:            "Reserved",
		typeErr:             "invalid encoding",
		outsideErr:          "other error,policy?",
		msgBigErr:           "msg is too big",
		extensionNotSupport: "server extension error",
		requestErr:          "unexcept error",
	}
)

//MessageHandler ...
type MessageHandler interface {
	HandleTextMessage(msg string, reply Handler) error
	HandleBinMessage(msg []byte, reply Handler) error
	HandleError(err []byte, reply Handler) error
	HandlePing(data []byte, reply Handler) error
	HandlePong(data []byte, reply Handler) error
	HandleClose(code uint16, data []byte) error
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
	fin     byte
	opcode  byte
	mask    byte
	pl      byte
	el      uint64
	mkey    []byte
	payload []byte
	rsv     byte
}

func (f *frame) toBytes() []byte {
	paylen := len(f.payload)
	data := []byte{(f.fin << 0x07) | (f.rsv << 0x04) | f.opcode}
	if paylen > payloadMaxLen {
		var buffer []byte
		if paylen > (1<<extLenFix)+payloadFixLen {
			f.pl = payloadMaxLen
			f.el = uint64(paylen - payloadMaxLen)
			binary.BigEndian.PutUint64(buffer, f.el)

		} else {
			f.pl = payloadFixLen
			f.el = uint64(paylen - payloadFixLen)
			binary.BigEndian.PutUint16(buffer, uint16(f.el))
		}
		data = append(data, (f.mask<<0x07)|f.pl)
		data = append(data, buffer...)

	} else {
		f.pl = byte(paylen)
		data = append(data, (f.mask<<0x07)|f.pl)
	}
	if f.mask == 1 {
		data = append(data, f.mkey[:4]...)
	}
	data = append(data, f.payload...)
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
	`, f.fin, f.opcode, f.mask, f.pl, f.el, f.mkey, f.payload, f.rsv)
}

func newFrame(fin, opcode, mask, rsv byte, mkey, payload []byte) *frame {
	return &frame{
		fin:     fin,
		opcode:  opcode,
		mask:    mask,
		rsv:     rsv,
		mkey:    mkey,
		payload: payload,
	}
}

type dhanler struct {
}

func (d *dhanler) HandleTextMessage(msg string, reply Handler) error {
	log.Printf("handle text data %s \n", msg)
	time.Sleep(time.Duration(2) * time.Second)
	return reply.Send([]byte("are u ok ?"))
}

func (d *dhanler) HandleBinMessage(msg []byte, reply Handler) error {
	log.Printf("handle binary data %s \n", msg)
	return reply.Send(alg.RandomMask())
}

func (d *dhanler) HandleError(err []byte, reply Handler) error {
	log.Printf("handle error message %s \n", string(err))
	return nil
}

func (d *dhanler) HandlePing(data []byte, reply Handler) error {
	log.Printf("handle ping message %s \n", string(data))
	return reply.Pong([]byte("i'm mibody!"))
}

func (d *dhanler) HandlePong(data []byte, reply Handler) error {
	log.Printf("handle pong message %s \n", string(data))
	return reply.Ping([]byte("are u ok?"))
}

func (d *dhanler) HandleClose(code uint16, data []byte) error {
	log.Printf("code is %d, reason is %s \n", code, string(data))
	return nil
}

type websocket struct {
	conn       net.Conn
	handler    MessageHandler
	frame      *frame
	extensions []string
}

//Handler ... simple interface for websocket impl
type Handler interface {
	//simple websocket method
	Ping(data []byte) error
	Pong(data []byte) error
	Send(msg []byte) error
	handleMessage() error
}

//NewHandler handler sub protocol
func NewHandler(conn net.Conn, handler MessageHandler, extensions []string) Handler {
	return &websocket{
		conn:       conn,
		handler:    handler,
		extensions: extensions,
	}
}

func (sc *websocket) enableDeflat() bool {
	for _, e := range sc.extensions {
		if strings.Contains(e, "permessage-deflate") {
			return true
		}
	}
	return false
}

func (sc *websocket) Ping(data []byte) error {
	f := newFrame(0, opp, 0, 0, nil, data)
	_, err := sc.conn.Write(f.toBytes())
	return err
}

func (sc *websocket) Pong(data []byte) error {
	f := newFrame(0, opg, 0, 0, nil, data)
	_, err := sc.conn.Write(f.toBytes())
	return err
}

func (sc *websocket) Send(msg []byte) error {
	content := msg
	var err error
	if sc.enableDeflat() {
		deflat := alg.Deflat{}
		content, err = deflat.Encoding(msg)
		if err != nil {
			log.Printf("encoding data failed")
			return err
		}
	}
	f := newFrame(sc.frame.fin, sc.frame.opcode, 0x0, sc.frame.rsv, nil, content)
	_, err = sc.conn.Write(f.toBytes())
	return err
}

func (sc *websocket) createFrame() (*frame, error) {
	//read maybe all frame
	var buffer = make([]byte, 1024)
	n, err := sc.conn.Read(buffer)
	if err != nil {
		return nil, err
	}
	f := &frame{}
	if n < 2 {
		sc.conn.Write(closeFrame(protocolErr, reasons[protocolErr]).toBytes())
		return nil, fmt.Errorf("error protocol")
	}
	setOpcode(buffer[0], f)
	//mask and len
	setMaskAndLen(buffer[1], f)
	payloadIdx := 2
	if f.pl == payloadFixLen {
		if n < 4 {
			sc.conn.Write(closeFrame(protocolErr, "invalid extlen exception").toBytes())
			return nil, fmt.Errorf("error ext len")
		}
		f.el, _ = binary.Uvarint(buffer[2:4])
		payloadIdx = 4
	} else if f.pl == payloadMaxLen {
		if n < 8 {
			sc.conn.Write(closeFrame(protocolErr, "invalid extlen exception").toBytes())
			return nil, fmt.Errorf("error ext len")
		}
		f.el, _ = binary.Uvarint(buffer[2:10])
		payloadIdx = 10
	}
	if f.mask == 1 {
		if n < payloadIdx+4 {
			sc.conn.Write(closeFrame(abnormal, "invalid mask len exception").toBytes())
			return nil, fmt.Errorf("invaild mask")
		}
		f.mkey = buffer[payloadIdx : payloadIdx+4]
		payloadIdx = payloadIdx + 4
	}
	//read data from connection
	totalLen := binary.BigEndian.Uint64([]byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, f.pl}) + f.el
	f.payload = buffer[payloadIdx:n]
	if uint64(n) < uint64(payloadIdx)+totalLen {
		rl := uint64(payloadIdx) + totalLen - uint64(n)
		var b = make([]byte, rl)
		n, err = io.ReadAtLeast(sc.conn, b, int(rl))
		if uint64(n) != rl {
			sc.conn.Write(closeFrame(outsideErr, "the message is to litte?").toBytes())
			return nil, fmt.Errorf("error data frame")
		}
		f.payload = append(f.payload, b...)
	}
	if err != nil {
		log.Printf("handle error data %s \n", err)
		return nil, err
	}
	if uint64(n-payloadIdx) != totalLen {
		log.Printf("read data error")
		return nil, err
	}
	if f.mask == 1 {
		alg.Translate(f.payload, f.mkey)
	}
	if sc.enableDeflat() && (f.opcode == opb || f.opcode == opt) {
		f.payload, _ = sc.decode(f.payload)
	}
	if err != nil {
		log.Printf("decode failed %s \n", err)
		sc.conn.Write(closeFrame(extensionNotSupport, reasons[extensionNotSupport]).toBytes())
		return nil, err
	}
	return f, nil
}

func (sc *websocket) handleMessage() error {
	for {
		f, err := sc.createFrame()
		if err != nil {
			sc.conn.Close()
			return err
		}
		sc.frame = f
		switch f.opcode {
		case opp:
			sc.handler.HandlePing(f.payload, sc)
			break
		case opg:
			sc.handler.HandlePong(f.payload, sc)
			break
		case opt:
			sc.handler.HandleTextMessage(string(f.payload), sc)
			break
		case opb:
			sc.handler.HandleBinMessage(f.payload, sc)
			break
		case ops:
			sc.handler.HandleClose(alg.ByteToUint16(f.payload[:2]), f.payload[2:])
			sc.close(normlClose)
			break
		default:
			sc.handler.HandleClose(alg.ByteToUint16(f.payload[:2]), f.payload[2:])
			sc.close(acceptErr)
			break
		}
	}
}
func (sc *websocket) decode(payload []byte) ([]byte, error) {
	deflat := alg.Deflat{}
	return deflat.Decoding(payload)
}

func (sc *websocket) close(closeCode int16) error {
	// bigEndian or littleEndian?
	_, err := sc.conn.Write(closeFrame(closeCode, "").toBytes())
	if err != nil {
		log.Printf("send close frame error %s \n", err)
	}
	return sc.conn.Close()
}

func closeFrame(code int16, reason string) *frame {
	return newFrame(0x0, ops, 0x0, 0x0, nil, append([]byte{byte(code >> 0x08), byte(code)}, []byte(reason)...))
}

func setMaskAndLen(segment byte, f *frame) {
	f.mask = (segment & 0x80) >> 0x07
	f.pl = segment & 0x7f
}

//set opcode ,fin and rsv
func setOpcode(segment byte, f *frame) {
	//one byte as one segment
	f.fin = (segment & 0x80) >> 0x07
	f.rsv = (segment & 0x70) >> 0x04
	f.opcode = segment & 0x0f
}
