import websocket
import time
import thread
def on_message(ws, message):
    print message

def on_error(ws, error):
    print error

def on_close(ws):
    print "### closed ###"

def on_open(ws):
    ws.send("hello world")
    ws.close()
if __name__ == '__main__':
    ws = websocket.WebSocket()
    ws.connect("ws://127.0.0.1:5000/test", on_close=on_close, on_message=on_message)
    ws.send(b"Hello world")
    time.sleep(100000)

