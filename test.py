import websocket
import time
import threading
import uuid

if __name__ == '__main__':
    ws = websocket.WebSocket()
    ws.connect("ws://127.0.0.1:5000/test")   
    t = threading.Thread()
    def run():
        while True:
            ws.send(uuid.uuid4().get_bytes(),opcode=websocket.ABNF.OPCODE_BINARY)
            data = ws.recv_data()
            print data
            time.sleep(1)
    t.run = run
    t.daemon = True
    t.start()
    time.sleep(20)
    ws.close()

