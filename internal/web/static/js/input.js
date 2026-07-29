/* Input.
 *
 * The keys are the game's own — WASD to move, YUBN for diagonals, Shift to run,
 * E to interact, Enter to chat — because someone who plays over SSH should not
 * have to learn a second set. What the browser adds is a real key-up event,
 * which the terminal has never had: SSH sees only a stream of repeats, so the
 * HD client has to guess when a held key was released. Here we know, so a held
 * key walks at an even cadence and stops the instant you let go.
 */

const MOVE_KEYS = {
  KeyW: 'w', KeyA: 'a', KeyS: 's', KeyD: 'd',
  ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right',
  KeyY: 'y', KeyU: 'u', KeyB: 'b', KeyN: 'n',
};

/* Keys forwarded straight to the area: interact, build-mode controls, hunting,
   taming, the overview map, slide navigation. */
const ACTION_KEYS = {
  KeyE: 'e', KeyR: 'r', KeyX: 'x', KeyF: 'f', KeyT: 't', KeyM: 'm',
  KeyP: 'p', BracketLeft: '[', BracketRight: ']',
};

const MOVE_INTERVAL = 100; // ms between steps while a key is held (10/s)

export class Input {
  constructor(ui, send) {
    this.ui = ui;
    this.send = send;
    this.held = new Set();
    this.lastMove = 0;
    this.lastDir = null;

    window.addEventListener('keydown', (e) => this.onKeyDown(e));
    window.addEventListener('keyup', (e) => this.onKeyUp(e));
    // Losing focus mid-stride would otherwise leave a key "held" forever.
    window.addEventListener('blur', () => this.held.clear());
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
      this.send({ t: 'key', key: ACTION_KEYS[e.code] });
    }
  }

  onKeyUp(e) {
    this.shift = e.shiftKey;
    this.held.delete(e.code);
  }

  /** step sends at most one movement per interval, newest direction winning. */
  step(now, immediate = false) {
    if (!this.held.size) return;
    if (!immediate && now - this.lastMove < MOVE_INTERVAL) return;
    // The most recently pressed key wins, so changing direction mid-walk turns
    // straight away instead of finishing the old direction first.
    const code = [...this.held][this.held.size - 1];
    let key = MOVE_KEYS[code];
    if (!key) return;
    if (this.shift) {
      // Running: Shift+arrow, or the uppercase letter, matching game.MoveKey.
      key = key.length === 1 ? key.toUpperCase() : 'shift+' + key;
    }
    this.send({ t: 'key', key });
    this.lastMove = now;
  }

  /** pump is called each frame to continue a held key. */
  pump(now) {
    this.step(now);
  }
}
