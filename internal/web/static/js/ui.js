/* The interface.
 *
 * Everything here is HTML over the WebGL canvas. That is the browser client's
 * one genuine advantage over the terminal ones: the HD renderer has no glyph
 * layer, so it draws the compendium, the trade table and the chat log into the
 * pixel frame by hand (most of internal/game/hd_ui.go). Here the server sends
 * the data and the DOM lays it out.
 *
 * The visual grammar is still docs/STYLE_GUIDE.md's: the world fills the
 * screen, the area name sits quietly top-left, and the bottom stays empty until
 * something is actually usable — then a prompt appears for it.
 *
 * Everything the server sends is treated as text, never as markup. Chat lines
 * and presentation decks are written by players; textContent is the rule.
 */

const $ = (id) => document.getElementById(id);

/** The Tab menu. Mirrors game.MenuEntries so the browser teaches the same
 *  shortcuts the terminal clients do. */
const MENU = [
  { label: 'Compendium', key: 'i', panel: 'compendium' },
  { label: 'Crafting', key: 'k', panel: 'craft' },
  { label: 'Character', key: 'c', panel: 'character' },
  { label: "Who's online", key: '', panel: 'who' },
  { label: 'Controls & help', key: '?', panel: 'help' },
];

export class UI {
  constructor(send) {
    this.send = send;
    this.el = {
      areaTitle: $('area-title'),
      claim: $('claim'),
      prompt: $('prompt'),
      toast: $('toast'),
      chatLog: $('chat-log'),
      chatInput: $('chat-input'),
      minimap: $('minimap'),
      build: $('build'),
      slide: $('slide'),
      hurt: $('hurt'),
      panel: $('panel'),
      panelTitle: $('panel-title'),
      panelRows: $('panel-rows'),
      panelFooter: $('panel-footer'),
      menu: $('menu'),
      menuRows: $('menu-rows'),
      status: $('status'),
    };
    this.chatActive = false;
    this.panelName = '';
    this.panelSel = 0;
    this.panelRowCount = 0;
    this.menuOpen = false;
    this.menuSel = 0;
    this.lastChatAt = 0;
    this.area = '';

    // The action camera's two overlays (docs/SWORDPLAY_PLAN.md): a reticle
    // dot, and a hint line teaching the combat grammar. Built here rather than
    // in the HTML because they exist only for that mode.
    const hud = document.getElementById('hud');
    this.reticle = document.createElement('div');
    this.reticle.id = 'reticle';
    this.reticle.hidden = true;
    hud.appendChild(this.reticle);
    this.actionHint = document.createElement('div');
    this.actionHint.id = 'action-hint';
    this.actionHint.textContent =
      'click swing · hold heavy · right-click guard · Space dodge · Q lock-on · V top-down';
    this.actionHint.hidden = true;
    hud.appendChild(this.actionHint);

    $('panel-close').addEventListener('click', () => this.closePanel());
    $('menu-close').addEventListener('click', () => this.closeMenu());
    this.el.panel.addEventListener('click', (e) => {
      if (e.target === this.el.panel) this.closePanel();
    });
    this.el.menu.addEventListener('click', (e) => {
      if (e.target === this.el.menu) this.closeMenu();
    });
    this.buildMenu();
  }

  /* ---------- the quiet chrome ---------- */

  setArea(name, flare) {
    if (name !== this.area) {
      this.area = name;
      this.el.areaTitle.textContent = name;
    }
    // The name flares on entry and settles — the same curve HD animates.
    this.el.areaTitle.classList.toggle('flare', (flare || 0) > 0.35);
  }

  setClaim(text) { this.el.claim.textContent = text || ''; }

  setPrompt(text) {
    const el = this.el.prompt;
    if (!text) { el.hidden = true; return; }
    el.textContent = text;
    el.hidden = false;
  }

  setToast(text) {
    const el = this.el.toast;
    if (!text) { el.hidden = true; return; }
    el.textContent = text;
    el.hidden = false;
  }

  /** setActionMode shows the duel overlays while the action camera is up. */
  setActionMode(on) {
    this.reticle.hidden = !on;
    this.actionHint.hidden = !on;
  }

  flashHurt(on) {
    this.el.hurt.classList.toggle('on', !!on);
    if (on) setTimeout(() => this.el.hurt.classList.remove('on'), 90);
  }

  setStatus(text, bad) {
    const el = this.el.status;
    el.textContent = text || '';
    el.classList.toggle('show', !!text);
    el.classList.toggle('bad', !!bad);
  }

  /* ---------- chat ---------- */

  addChat(msg) {
    if (msg.kind === 'clear') { this.el.chatLog.replaceChildren(); return; }
    const line = document.createElement('div');
    line.className = 'line ' + (msg.kind || 'system');
    line.textContent = msg.text; // never innerHTML: players write this
    if (msg.hex) line.style.color = msg.hex;
    this.el.chatLog.appendChild(line);
    while (this.el.chatLog.children.length > 40) {
      this.el.chatLog.removeChild(this.el.chatLog.firstChild);
    }
    this.lastChatAt = performance.now();
    this.el.chatLog.classList.remove('idle');
  }

  openChat(prefill = '') {
    this.chatActive = true;
    this.el.chatInput.hidden = false;
    this.el.chatInput.value = prefill;
    this.el.chatInput.focus();
    this.el.chatLog.classList.remove('idle');
  }

  closeChat() {
    this.chatActive = false;
    this.el.chatInput.hidden = true;
    this.el.chatInput.value = '';
    this.el.chatInput.blur();
  }

  submitChat() {
    const text = this.el.chatInput.value.trim();
    this.closeChat();
    if (text) this.send({ t: 'chat', text });
  }

  /** tickChat fades the log when nothing has been said for a while. */
  tickChat(now) {
    if (this.chatActive) return;
    this.el.chatLog.classList.toggle('idle', now - this.lastChatAt > 12000);
  }

  /* ---------- minimap, build palette, slides ---------- */

  setMinimap(mm) {
    const el = this.el.minimap;
    if (!mm) { el.hidden = true; return; }
    el.hidden = false;
    el.querySelector('.minimap-title').textContent = mm.title || '';
    const canvas = el.querySelector('.minimap-canvas');
    const ctx = canvas.getContext('2d');
    const rows = mm.rows || [];
    const h = rows.length, w = h ? rows[0].length : 0;
    if (!w || !h) return;
    const cell = Math.max(1, Math.floor(Math.min(canvas.width / w, canvas.height / h)));
    ctx.fillStyle = '#0b0e12';
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    for (let y = 0; y < h; y++) {
      for (let x = 0; x < w; x++) {
        const hex = rows[y][x];
        if (!hex) continue; // unexplored: leave it dark
        ctx.fillStyle = hex;
        ctx.fillRect(x * cell, y * cell, cell, cell);
      }
    }
    if (mm.sx >= 0) {
      ctx.fillStyle = '#ffffff';
      ctx.fillRect(mm.sx * cell, mm.sy * cell, Math.max(2, cell), Math.max(2, cell));
    }
  }

  setBuild(build) {
    const el = this.el.build;
    if (!build) { el.hidden = true; return; }
    el.hidden = false;
    const list = el.querySelector('.build-list');
    list.replaceChildren();
    (build.items || []).forEach((name, i) => {
      const li = document.createElement('li');
      li.textContent = name;
      if (i === build.sel) li.className = 'sel';
      list.appendChild(li);
    });
    const foot = el.querySelector('.build-footer');
    foot.textContent = build.footer || 'r next · e place · x remove · b done';
    foot.classList.toggle('warn', !!build.warn);
  }

  setSlide(slide) {
    const el = this.el.slide;
    if (!slide) { el.hidden = true; return; }
    el.hidden = false;
    el.querySelector('.slide-body').replaceChildren(...renderMarkdown(slide.src || ''));
    el.querySelector('.slide-footer').textContent = slide.footer || '';
  }

  /* ---------- panels ---------- */

  buildMenu() {
    this.el.menuRows.replaceChildren();
    MENU.forEach((entry, i) => {
      const li = document.createElement('li');
      li.className = 'actionable';
      const label = document.createElement('span');
      label.textContent = entry.label;
      const key = document.createElement('span');
      key.className = 'value';
      key.textContent = entry.key;
      li.append(label, key);
      li.addEventListener('click', () => {
        this.closeMenu();
        this.openPanel(entry.panel);
      });
      this.el.menuRows.appendChild(li);
    });
  }

  openMenu() {
    this.menuOpen = true;
    this.menuSel = 0;
    this.el.menu.hidden = false;
    this.highlightMenu();
  }

  closeMenu() {
    this.menuOpen = false;
    this.el.menu.hidden = true;
  }

  moveMenu(d) {
    this.menuSel = (this.menuSel + d + MENU.length) % MENU.length;
    this.highlightMenu();
  }

  highlightMenu() {
    [...this.el.menuRows.children].forEach((li, i) => {
      li.classList.toggle('sel', i === this.menuSel);
    });
  }

  chooseMenu() {
    const entry = MENU[this.menuSel];
    this.closeMenu();
    if (entry) this.openPanel(entry.panel);
  }

  openPanel(name) {
    this.send({ t: 'panel', panel: name, sel: 0 });
  }

  closePanel() {
    if (this.panelName === 'trade') {
      // Walking away from a trade table has to cancel it for both sides, so
      // closing the panel is a real action, not just hiding a window.
      this.send({ t: 'panel', panel: 'trade', act: 'cancel', sel: this.panelSel });
    }
    this.panelName = '';
    this.el.panel.hidden = true;
    this.send({ t: 'panel', panel: '' });
  }

  /** showPanel renders a PanelMsg. */
  showPanel(msg) {
    if (!msg.panel) {
      this.panelName = '';
      this.el.panel.hidden = true;
      return;
    }
    this.panelName = msg.panel;
    this.panelSel = msg.sel || 0;
    this.el.panel.hidden = false;
    this.el.panelTitle.textContent = msg.title || '';
    this.el.panelFooter.textContent = msg.footer || '';

    const rows = msg.rows || [];
    this.panelRowCount = rows.length;
    const frag = document.createDocumentFragment();
    rows.forEach((row, i) => {
      const li = document.createElement('li');
      const isHead = !row.value && !row.desc && row.hex && !row.label.startsWith(' ');
      const classes = [];
      // A row with only a label and an accent color is a group heading.
      if (isHead && looksLikeHeading(row)) classes.push('head');
      if (row.dim) classes.push('dim');
      if (row.warn) classes.push('warn');
      if (row.sel) { classes.push('sel'); this.panelSel = i; }
      if (isSelectable(msg.panel)) classes.push('actionable');
      li.className = classes.join(' ');

      const label = document.createElement('span');
      label.textContent = row.label;
      if (row.hex && !classes.includes('head')) label.style.color = row.hex;
      li.appendChild(label);

      if (row.value) {
        const val = document.createElement('span');
        val.className = 'value';
        val.textContent = row.value;
        li.appendChild(val);
      }
      if (row.desc) {
        const desc = document.createElement('span');
        desc.className = 'desc';
        desc.textContent = row.desc;
        li.appendChild(desc);
      }
      if (isSelectable(msg.panel)) {
        li.addEventListener('click', () => {
          this.send({ t: 'panel', panel: msg.panel, sel: i });
        });
        li.addEventListener('dblclick', () => {
          this.send({ t: 'panel', panel: msg.panel, sel: i, act: 'use' });
        });
      }
      frag.appendChild(li);
    });
    this.el.panelRows.replaceChildren(frag);
    this.scrollToSelection();
  }

  scrollToSelection() {
    const sel = this.el.panelRows.querySelector('.sel');
    sel?.scrollIntoView({ block: 'nearest' });
  }

  /** movePanel changes the selection in a list panel. */
  movePanel(d) {
    if (!this.panelRowCount) return;
    const next = Math.max(0, Math.min(this.panelRowCount - 1, this.panelSel + d));
    this.panelSel = next;
    this.send({ t: 'panel', panel: this.panelName, sel: next });
  }

  panelAction(act) {
    if (!this.panelName) return;
    this.send({ t: 'panel', panel: this.panelName, sel: this.panelSel, act });
  }

  get panelOpen() { return !!this.panelName; }
  get anyModal() { return this.panelOpen || this.menuOpen || this.chatActive; }
}

/** Panels whose rows are a list you pick from, rather than a reference to read. */
function isSelectable(panel) {
  return panel === 'craft' || panel === 'stall' || panel === 'trade' || panel === 'character';
}

function looksLikeHeading(row) {
  return !row.value && !row.desc;
}

/* ---------- a very small markdown renderer ----------
 *
 * Presentation decks are player-authored markdown. The server sends the source
 * and this turns it into DOM nodes — nodes, not an HTML string, so there is no
 * path by which a deck can inject markup into anyone else's page. It covers the
 * subset that reads well on a slide; anything else falls back to plain text.
 */
function renderMarkdown(src) {
  const out = [];
  const lines = src.split('\n');
  let list = null, code = null;

  const endList = () => { if (list) { out.push(list); list = null; } };

  for (const raw of lines) {
    const line = raw.replace(/\s+$/, '');

    if (code !== null) {
      if (line.trim().startsWith('```')) {
        const pre = document.createElement('pre');
        const el = document.createElement('code');
        el.textContent = code.join('\n');
        pre.appendChild(el);
        out.push(pre);
        code = null;
      } else {
        code.push(raw);
      }
      continue;
    }
    if (line.trim().startsWith('```')) { endList(); code = []; continue; }

    const heading = /^(#{1,4})\s+(.*)$/.exec(line);
    if (heading) {
      endList();
      const h = document.createElement('h' + heading[1].length);
      h.append(...inline(heading[2]));
      out.push(h);
      continue;
    }
    if (/^\s*[-*]\s+/.test(line)) {
      if (!list) list = document.createElement('ul');
      const li = document.createElement('li');
      li.append(...inline(line.replace(/^\s*[-*]\s+/, '')));
      list.appendChild(li);
      continue;
    }
    if (/^\s*>\s?/.test(line)) {
      endList();
      const bq = document.createElement('blockquote');
      bq.append(...inline(line.replace(/^\s*>\s?/, '')));
      out.push(bq);
      continue;
    }
    if (/^\s*(---|\*\*\*)\s*$/.test(line)) { endList(); out.push(document.createElement('hr')); continue; }
    if (!line.trim()) { endList(); continue; }

    endList();
    const p = document.createElement('p');
    p.append(...inline(line));
    out.push(p);
  }
  endList();
  if (code) {
    const pre = document.createElement('pre');
    pre.textContent = code.join('\n');
    out.push(pre);
  }
  return out;
}

/** inline handles bold, italic, strikethrough and code spans, emitting nodes. */
function inline(text) {
  const nodes = [];
  const re = /(\*\*[^*]+\*\*|__[^_]+__|\*[^*]+\*|_[^_]+_|~~[^~]+~~|`[^`]+`)/g;
  let last = 0, m;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) nodes.push(document.createTextNode(text.slice(last, m.index)));
    const tok = m[0];
    let el;
    if (tok.startsWith('**') || tok.startsWith('__')) {
      el = document.createElement('strong');
      el.textContent = tok.slice(2, -2);
    } else if (tok.startsWith('~~')) {
      el = document.createElement('s');
      el.textContent = tok.slice(2, -2);
    } else if (tok.startsWith('`')) {
      el = document.createElement('code');
      el.textContent = tok.slice(1, -1);
    } else {
      el = document.createElement('em');
      el.textContent = tok.slice(1, -1);
    }
    nodes.push(el);
    last = m.index + tok.length;
  }
  if (last < text.length) nodes.push(document.createTextNode(text.slice(last)));
  return nodes;
}
