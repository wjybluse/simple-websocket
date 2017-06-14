import websocket
import time
import threading
if __name__ == '__main__':
    ws = websocket.WebSocket()
    ws.connect("ws://127.0.0.1:5000/test")
    ws.send(b"Hello world")
    data = ws.recv_data()
    t = threading.Thread()
    def run():
        while True:
            ws.ping("mibody")
            time.sleep(1)
    t.run = run
    t.daemon =True
    t.start()
    time.sleep(5)
    print data
    ws.close()

