/* The connection.
 *
 * One websocket carrying JSON both ways. The server is authoritative about
 * everything — where you are, what you're holding, whether that gate opened —
 * so this module never predicts, only relays. If the socket drops, the game
 * pauses and reconnects; the world kept turning without us and the next scene
 * message tells us the truth.
 */

export class Connection {
  constructor(name, handlers) {
    this.name = name;
    this.handlers = handlers;
    this.ws = null;
    this.closed = false;
    this.retries = 0;
    this.queue = [];
  }

  url() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${proto}//${location.host}/ws?name=${encodeURIComponent(this.name)}`;
  }

  connect() {
    if (this.closed) return;
    const ws = new WebSocket(this.url());
    this.ws = ws;

    ws.onopen = () => {
      this.retries = 0;
      this.handlers.onStatus?.('connected');
      for (const m of this.queue) ws.send(m);
      this.queue.length = 0;
    };

    ws.onmessage = (ev) => {
      let msg;
      try {
        msg = JSON.parse(ev.data);
      } catch {
        return; // a message we can't parse is a message we can't act on
      }
      this.handlers.onMessage?.(msg);
    };

    ws.onclose = (ev) => {
      if (this.closed) return;
      // 1008 is the server refusing us (a bad name, usually): retrying would
      // just fail the same way, so surface it instead of looping.
      if (ev.code === 1008) {
        this.closed = true;
        this.handlers.onFatal?.(ev.reason || 'the server refused the connection');
        return;
      }
      this.retries++;
      const wait = Math.min(8000, 400 * Math.pow(2, this.retries - 1));
      this.handlers.onStatus?.('reconnecting');
      setTimeout(() => this.connect(), wait);
    };

    ws.onerror = () => { /* onclose follows and handles the retry */ };
  }

  send(obj) {
    const data = JSON.stringify(obj);
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(data);
    } else if (this.queue.length < 32) {
      // Hold a little input across a blip rather than dropping it silently.
      this.queue.push(data);
    }
  }

  close() {
    this.closed = true;
    this.ws?.close(1000, 'bye');
  }
}
