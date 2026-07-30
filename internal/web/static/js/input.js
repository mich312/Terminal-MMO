/* Input.
 *
 * The keys are the game's own — WASD to move, YUBN for diagonals, Shift to run,
 * E to interact, Enter to chat — because someone who plays over SSH should not
 * have to learn a second set. What the browser adds is a real key-up event,
 * which the terminal has never had: SSH sees only a stream of repeats, so the
 * HD client has to guess when a held key was released. Here we know, so a held
 * key walks at an even cadence and stops the instant you let go.
 *
 * Movement is a steering vector rather than a key. The held keys make a
 * direction — camera-relative in action mode, so walking at 20° off the camera
 * really is 20° — and that vector goes on the wire. It used to be rounded to
 * one of eight compass keys the line after it was computed, because a key
 * string was the only thing the client could send; the server now takes the
 * vector (protocol v4) and the body walks along it under its own power.
 *
 * V toggles the action camera (docs/SWORDPLAY_PLAN.md), and with it the rest of
 * that grammar: the mouse swings (tap = fast, hold = strong) and guards (right
 * button), Space dodge-rolls, Q locks on. Every verb still goes through the
 * server's referee — this file only decides what to send, and animates
 * hopefully while the ruling comes back.
 */

const MOVE_KEYS = {
  KeyW: 'w', KeyA: 'a', KeyS: 's', KeyD: 'd',
  ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right',
  KeyY: 'y', KeyU: 'u', KeyB: 'b', KeyN: 'n',
};

/* Keys forwarded straight to the area: interact, build-mode controls, striking,
   taming, the overview map, slide navigation. */
const ACTION_KEYS = {
  KeyE: 'e', KeyR: 'r', KeyX: 'x', KeyF: 'f', KeyT: 't', KeyM: 'm',
  KeyP: 'p', BracketLeft: '[', BracketRight: ']',
};

/* Camera-relative movement, action mode only: which held keys push which way
   in the camera's frame. YUBN keep their absolute grid meaning everywhere. */
const REL_KEYS = {
  KeyW: [0, 1], ArrowUp: [0, 1],
  KeyS: [0, -1], ArrowDown: [0, -1],
  KeyA: [-1, 0], ArrowLeft: [-1, 0],
  KeyD: [1, 0], ArrowRight: [1, 0],
};

/* Each movement key as a world-axis direction (x east, y south), so the grid
   keys and the roguelike diagonals can be summed into a steering vector like
   any other input. */
const KEY_VEC = {
  w: [0, -1], up: [0, -1],
  s: [0, 1], down: [0, 1],
  a: [-1, 0], left: [-1, 0],
  d: [1, 0], right: [1, 0],
  y: [-1, -1], u: [1, -1], b: [-1, 1], n: [1, 1],
};

const MOVE_INTERVAL = 100;  // ms between re-asserting a held steer (see game.IntentTTL)
const STRONG_HOLD_MS = 300; // holding the swing past this winds up the heavy blow
const FACE_INTERVAL = 90;   // ms between facing updates sent to the server

/** dirIndex quantizes a ground-plane vector to world.Dir (S=+z, E=+x). */
function dirIndex(dx, dz) {
  const a = Math.atan2(dx, dz);
  return ((Math.round(a / (Math.PI / 4)) % 8) + 8) % 8;
}

export class Input {
  constructor(ui, send) {
    this.ui = ui;
    this.send = send;
    this.held = new Set();
    this.lastMove = 0;
    this.lastDir = null;
    // The last steering vector the server was told, so a held key is only
    // re-sent on the keepalive rather than every frame.
    this.sentDX = 0; this.sentDY = 0; this.sentRun = false;

    this.mode = 'top';
    this.hooks = null; // {scene, actors, toggleView} — wired by main
    this.pressAt = 0;
    this.strongSent = false;
    this.guarding = false;
    this.lastFaceIdx = -1;
    this.lastFaceAt = 0;

    window.addEventListener('keydown', (e) => this.onKeyDown(e));
    window.addEventListener('keyup', (e) => this.onKeyUp(e));
    // Losing focus mid-stride would otherwise leave a key "held" forever —
    // or a guard raised, which in a duel is worse.
    window.addEventListener('blur', () => {
      this.held.clear();
      this.releaseGuard();
    });
    window.addEventListener('mousedown', (e) => this.onMouseDown(e));
    window.addEventListener('mouseup', (e) => this.onMouseUp(e));
  }

  /** attach hands over the scene and actors, once main has built them. */
  attach(hooks) {
    this.hooks = hooks;
  }

  setMode(mode) {
    this.mode = mode;
    this.releaseGuard();
    this.pressAt = 0;
    this.lastFaceIdx = -1;
    if (mode !== 'action') this.hooks?.actors.setLock(null);
  }

  onKeyDown(e) {
    this.shift = e.shiftKey;
    // While typing a chat line, the keyboard belongs to the input field.
    if (this.ui.chatActive) {
      if (e.key === 'Enter') { e.preventDefault(); this.ui.submitChat(); }
      else if (e.key === 'Escape') { e.preventDefault(); this.ui.closeChat(); }
      return;
    }
    if (e.metaKey || e.ctrlKey || e.altKey) return;

    if (this.ui.menuOpen) {
      e.preventDefault();
      switch (e.key) {
        case 'ArrowUp': this.ui.moveMenu(-1); break;
        case 'ArrowDown': this.ui.moveMenu(1); break;
        case 'Enter': this.ui.chooseMenu(); break;
        case 'Escape': case 'Tab': this.ui.closeMenu(); break;
      }
      return;
    }

    if (this.ui.panelOpen) {
      e.preventDefault();
      switch (e.key) {
        case 'Escape': this.ui.closePanel(); break;
        case 'ArrowUp': this.ui.movePanel(-1); break;
        case 'ArrowDown': this.ui.movePanel(1); break;
        case 'ArrowLeft': this.ui.panelAction('prev'); break;
        case 'ArrowRight': this.ui.panelAction('next'); break;
        case 'Enter': this.ui.panelAction('use'); break;
        case '+': case '=': this.ui.panelAction('add'); break;
        case '-': case '_': this.ui.panelAction('sub'); break;
        case 'r': this.ui.panelAction('ready'); break;
        case 'f': this.ui.panelAction('fuel'); break;
        case 'x': this.ui.panelAction('remove'); break;
        // A panel swallows everything else, so a stray key can't walk you off
        // while you're reading the compendium.
      }
      return;
    }

    // Opening things.
    switch (e.key) {
      case 'Enter': e.preventDefault(); this.ui.openChat(); return;
      case '/': e.preventDefault(); this.ui.openChat('/'); return;
      case 'Tab': e.preventDefault(); this.ui.openMenu(); return;
      case '?': e.preventDefault(); this.ui.openPanel('help'); return;
      case 'i': e.preventDefault(); this.ui.openPanel('compendium'); return;
      case 'c': e.preventDefault(); this.ui.openPanel('character'); return;
      case 'k': e.preventDefault(); this.ui.openPanel('craft'); return;
    }

    // The view toggle, and the action-mode combat keys.
    if (e.code === 'KeyV') {
      e.preventDefault();
      this.hooks?.toggleView();
      return;
    }
    if (this.mode === 'action') {
      if (e.code === 'Space') {
        e.preventDefault();
        if (!e.repeat) this.dodge();
        return;
      }
      if (e.code === 'KeyQ') {
        e.preventDefault();
        if (!e.repeat) this.toggleLock();
        return;
      }
      if (e.code === 'KeyF' && this.shift) {
        e.preventDefault();
        this.send({ t: 'key', key: 'F' });
        this.hooks?.actors.localAct('strong');
        return;
      }
    }

    if (MOVE_KEYS[e.code]) {
      e.preventDefault();
      this.held.add(e.code);
      // Step immediately so a tap is responsive; the pump below handles the
      // even cadence of a held key.
      this.step(performance.now(), true);
      return;
    }
    if (ACTION_KEYS[e.code]) {
      e.preventDefault();
      const key = ACTION_KEYS[e.code];
      this.send({ t: 'key', key });
      if (this.mode === 'action' && key === 'f') this.hooks?.actors.localAct('fast');
    }
  }

  onKeyUp(e) {
    this.shift = e.shiftKey;
    if (this.held.delete(e.code)) {
      // Tell the server we stopped now rather than letting the intent time out
      // — a browser has real key-up events and should spend them.
      this.step(performance.now(), true);
    }
  }

  onMouseDown(e) {
    if (this.mode !== 'action' || this.ui.chatActive || this.ui.panelOpen || this.ui.menuOpen) return;
    switch (e.button) {
      case 0: // the sword hand: tap = fast, hold = strong (see pump)
        // Without pointer lock the camera can't follow the mouse, so the
        // first click buys the lock back rather than swinging blind.
        if (!document.pointerLockElement) {
          this.hooks?.scene.relock();
          return;
        }
        this.pressAt = performance.now();
        this.strongSent = false;
        break;
      case 1: // middle: lock-on, same as Q
        e.preventDefault();
        this.toggleLock();
        break;
      case 2: // the off hand: guard while held
        this.guarding = true;
        this.send({ t: 'key', key: 'guard:1' });
        if (this.hooks?.actors.self) this.hooks.actors.self.guarding = true;
        break;
    }
  }

  onMouseUp(e) {
    if (e.button === 2) {
      this.releaseGuard();
      return;
    }
    if (e.button !== 0 || this.mode !== 'action' || !this.pressAt) return;
    const held = performance.now() - this.pressAt;
    this.pressAt = 0;
    if (this.strongSent) return; // the wind-up already loosed the heavy blow
    if (held < STRONG_HOLD_MS) {
      this.send({ t: 'key', key: 'f' });
      this.hooks?.actors.localAct('fast');
    }
  }

  releaseGuard() {
    if (!this.guarding) return;
    this.guarding = false;
    this.send({ t: 'key', key: 'guard:0' });
    if (this.hooks?.actors.self) this.hooks.actors.self.guarding = false;
  }

  dodge() {
    // Roll the way you're steering; standing still, roll the way you're looking.
    const idx = this.moveDir() ?? this.aimDir();
    if (idx == null) return;
    this.send({ t: 'key', key: 'dodge:' + idx });
    this.hooks?.actors.localAct('dodge');
  }

  toggleLock() {
    const a = this.hooks?.actors;
    if (!a) return;
    if (a.lockName) { a.setLock(null); return; }
    const self = a.self;
    if (!self) return;
    const f = this.hooks.scene.forward();
    a.setLock(a.nearestTarget(self.x, self.z, f.x, f.z));
  }

  /** moveDir is the held steering quantized to a world.Dir index, or null.
   *  Walking no longer needs this — it sends the vector itself — but the dodge
   *  roll is still one of eight, so it keeps a use. */
  moveDir() {
    const v = this.steerVector();
    return v ? dirIndex(v.dx, v.dy) : null;
  }

  /** aimDir is where the camera looks (or the lock target stands), quantized. */
  aimDir() {
    const hooks = this.hooks;
    if (!hooks) return null;
    const a = hooks.actors;
    if (a.lockName && a.self) {
      const t = a.players.get(a.lockName);
      if (t) return dirIndex(t.x - a.self.x, t.z - a.self.z);
    }
    const f = hooks.scene.forward();
    return dirIndex(f.x, f.z);
  }

  /** steerVector is where the held keys are pushing, in world axes (x east,
   *  y south), or null when nothing is held.
   *
   *  In action mode this is genuinely analog: the keys make a vector in the
   *  camera's frame and it is rotated into the world's, so walking at 20° off
   *  the camera means walking at 20°. It used to be rounded to one of eight
   *  compass directions right here — the server could only be told a movement
   *  key, so seven eighths of the information was thrown away a line after it
   *  was computed. */
  steerVector() {
    let mx = 0, mz = 0;
    if (this.mode === 'action') {
      for (const code of this.held) {
        const rel = REL_KEYS[code];
        if (rel) { mx += rel[0]; mz += rel[1]; }
      }
      if (mx || mz) {
        const f = this.hooks.scene.forward();
        return { dx: f.x * mz + -f.z * mx, dy: f.z * mz + f.x * mx };
      }
      // Only the roguelike diagonals held: they keep their absolute meaning.
    }
    // Top-down, and the diagonals in action mode: the grid keys are directions
    // in their own right. Summing them gives the diagonals for free.
    for (const code of this.held) {
      const g = KEY_VEC[MOVE_KEYS[code]];
      if (g) { mx += g[0]; mz += g[1]; }
    }
    return (mx || mz) ? { dx: mx, dy: mz } : null;
  }

  /** step sends the steering vector when it changes, and repeats it while the
   *  keys are held so the server's intent doesn't go stale (game.IntentTTL).
   *  A key-up sends a zero vector at once, so letting go stops you now rather
   *  than a quarter-second later. */
  step(now, immediate = false) {
    const v = this.steerVector();
    const dx = v ? v.dx : 0, dy = v ? v.dy : 0;
    const run = v ? !!this.shift : false;
    const changed = dx !== this.sentDX || dy !== this.sentDY || run !== this.sentRun;
    if (!changed && !v) return;                                    // idle: nothing to say
    if (!changed && !immediate && now - this.lastMove < MOVE_INTERVAL) return;
    this.sentDX = dx; this.sentDY = dy; this.sentRun = run;
    this.lastMove = now;
    this.send({ t: 'move', dx, dy, run });
  }

  /** pump is called each frame: continue a held key, keep the server's facing
   *  pointed where the action camera aims, and loose a held-down heavy blow. */
  pump(now) {
    this.step(now);
    if (this.mode !== 'action') return;

    // Facing follows the aim (the lock target wins over the camera), so a
    // strike goes where you look — on an 8-way grid, the difference between
    // fencing and a slot machine.
    const idx = this.aimDir();
    if (idx != null && idx !== this.lastFaceIdx && now - this.lastFaceAt >= FACE_INTERVAL) {
      this.lastFaceIdx = idx;
      this.lastFaceAt = now;
      this.send({ t: 'key', key: 'face:' + idx });
    }

    // A held swing crosses the threshold into the strong attack: the wind-up
    // starts now, visibly, and the server hears the heavier verb.
    if (this.pressAt && !this.strongSent && now - this.pressAt >= STRONG_HOLD_MS) {
      this.strongSent = true;
      this.send({ t: 'key', key: 'F' });
      this.hooks?.actors.localAct('strong');
    }
  }
}
