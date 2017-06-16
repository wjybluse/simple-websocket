package server

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/elians/websocket/utils"
)

const (
	guid = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	p    = "chat"
)

const (
	requestHeader  = "GET %s HTTP/1.1"
	responseHeader = "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept:%s\r\n"
	badRequest     = "HTTP/1.1 400 Bad Request\r\n"
)

type websocket struct {
	host string
	//websocket location
	url string
}
type header struct {
	name, value string
}

//Header ... new type define
type Header struct {
	headers map[string][]string
	lock    *sync.RWMutex
}

//NewHeader creat header
func NewHeader() *Header {
	return &Header{
		headers: make(map[string][]string, 0),
		lock:    new(sync.RWMutex),
	}
}

//Get value
func (hs Header) Get(key string) []string {
	defer hs.lock.Unlock()
	hs.lock.Lock()
	return hs.headers[key]
}

//Put value
func (hs Header) Put(key, value string) {
	v := hs.headers[key]
	defer hs.lock.Unlock()
	hs.lock.Lock()
	if v != nil {
		hs.headers[key] = append(v, value)
	} else {
		hs.headers[key] = []string{value}
	}
}

//PutAll put all header
func (hs Header) PutAll(pairs map[string]string) {
	defer hs.lock.Unlock()
	hs.lock.Lock()
	for key, value := range pairs {
		v := hs.headers[key]
		if v == nil {
			hs.headers[key] = []string{}
		}
		hs.headers[key] = append(hs.headers[key], value)
	}
}

//RequestURL ...
func (hs Header) RequestURL() string {
	if hs.headers["Request-URL"] == nil {
		return ""
	}
	return hs.headers["Request-URL"][0]
}

//Method get method
func (hs Header) Method() string {
	if hs.headers["method"] == nil {
		return ""
	}
	return hs.headers["method"][0]
}

//Extensions websocket extension
func (hs Header) Extensions() []string {
	return hs.headers["Sec-WebSocket-Extensions"]
}

//HTTPVersion hhtp version
func (hs Header) HTTPVersion() string {
	if hs.headers["HTTP-Version"] == nil {
		return ""
	}
	return hs.headers["HTTP-Version"][0]
}

type protocol interface {
	handshake(headers Header) error
	upgrade(conn net.Conn, headers Header) error
}

//Listener ...listener interface
type Listener interface {
	Listen() error
}

//NewWebsocket ... return new websocket
func NewWebsocket(host, url string) Listener {
	return &websocket{
		host: host,
		url:  url,
	}
}

func (w *websocket) Listen() error {
	listen, err := net.Listen("tcp", w.host)
	if err != nil {
		return err
	}
	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Printf("error connection handler,%s", err)
			continue
		}
		go w.handleConn(conn)
	}
}

func (w *websocket) handleConn(conn net.Conn) error {
	var buf = make([]byte, 7*1024)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	if n < 7*1024 {
		buf = buf[:n]
	}
	header := w.parserHeaders(buf)
	err = w.handshake(header)
	if err != nil {
		defer conn.Close()
		if strings.Contains(err.Error(), "version") {
			//write bad request header with invalid version
			conn.Write([]byte(badRequest + "Sec-WebSocket-Version:13,8,7\r\n\r\n"))
		} else {
			conn.Write([]byte(badRequest + "\r\n"))
		}

		fmt.Printf("handle error is %s \n", err)
		return err
	}
	cid := header.Get("Sec-WebSocket-Key")[0]
	accept := utils.Base64Encoding(utils.Sha1(cid + guid))
	extensions := header.Get("Sec-WebSocket-Extensions")
	rsp := fmt.Sprintf(responseHeader+"\r\n", accept)
	if len(extensions) > 0 {
		ext := extensions[0]
		if strings.Contains(ext, "client_max_window_bits") {
			ext = strings.Replace(ext, "client_max_window_bits", "client_max_window_bits=15", -1)
		}
		rsp = fmt.Sprintf(responseHeader+"%s\r\n\r\n", accept, "Sec-WebSocket-Extensions:"+ext)
	}
	conn.Write([]byte(rsp))
	sub := NewHandler(conn, &dhanler{}, extensions)
	sub.handleMessage()
	return nil
}

func (w *websocket) parserHeaders(buf []byte) *Header {
	content := string(buf)
	lines := strings.Split(content, "\r\n")
	if len(lines) < 3 {
		return nil
	}
	arr := strings.Split(lines[0], " ")
	if len(arr) < 3 {
		return nil
	}
	header := NewHeader()
	header.Put("method", arr[0])
	header.Put("Request-URL", arr[1])
	header.Put("HTTP-Version", arr[2])
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		pair := strings.Split(line, ":")
		if len(pair) < 2 {
			continue
		}
		header.Put(strings.TrimSpace(pair[0]), strings.TrimSpace(pair[1]))
	}
	return header
}

func (w *websocket) handshake(header *Header) error {
	if header.Method() != "GET" {
		//TODO method invalid
		return fmt.Errorf("error method")
	}
	if w.url != header.RequestURL() {
		return fmt.Errorf("invalid path")
	}
	upgrade := header.Get("Upgrade")
	if upgrade == nil || len(upgrade) <= 0 {
		return fmt.Errorf("upgrade error")
	}
	connections := header.Get("Connection")
	if len(connections) <= 0 || connections[0] != "Upgrade" {
		return fmt.Errorf("upgrade error")
	}
	if upgrade[0] != "websocket" {
		return fmt.Errorf("upgrade error")
	}
	key := header.Get("Sec-WebSocket-Key")
	//may include
	//pt := headers.Get("Sec-WebSocket-Protocol")
	version := header.Get("Sec-WebSocket-Version")
	if key == nil || len(key) == 0 || key[0] == "" {
		return fmt.Errorf("key is invalid")
	}
	if version == nil || len(version) <= 0 || version[0] == "" || version[0] != "13" {
		return fmt.Errorf("version is invalid")
	}
	return nil
}
