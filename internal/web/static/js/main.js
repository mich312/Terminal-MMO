/* Durst World — browser client.
 *
 * Boot order: take a name, open the socket, learn the shape vocabulary from the
 * server's hello, then run a frame loop that draws whatever the last scene
 * message said. The client is a renderer and an input device; the server owns
 * the world. Nothing here decides whether you can walk somewhere or whether
 * that berry went into your pack.
 */

import { WorldScene } from './scene.js';
import { TileField } from './field.js';
import { ActorField } from './actors.js';
import { Connection } from './net.js';
import { UI } from './ui.js';
import { Input } from './input.js';

const NAME_KEY = 'durstworld.name';
const PROTOCOL = 4;

const gate = document.getElementById('gate');
const gameEl = document.getElementById('game');
const form = document.getElementById('join-form');
const nameInput = document.getElementById('join-name');
const joinButton = document.getElementById('join-go');
const gateError = document.getElementById('gate-error');

// Your name is remembered so you come back as the same character — the browser
// equivalent of an SSH username, which is the one thing a browser doesn't
// bring with it.
nameInput.value = localStorage.getItem(NAME_KEY) || '';

form.addEventListener('submit', (e) => {
  e.preventDefault();
  const name = nameInput.value.trim();
  if (!/^[A-Za-z0-9_-]{1,16}$/.test(name)) {
    showGateError('A name of 1–16 letters, digits, - or _ please.');
    return;
  }
  localStorage.setItem(NAME_KEY, name);
  joinButton.disabled = true;
  joinButton.textContent = 'Walking in…';
  start(name);
});

function showGateError(text) {
  gateError.textContent = text;
  gateError.hidden = false;
  joinButton.disabled = false;
  joinButton.textContent = 'Walk in';
}

function start(name) {
  let scene;
  try {
    scene = new WorldScene(document.getElementById('view'));
  } catch (err) {
    showGateError('This browser could not start WebGL. ' +
      'You can still play over SSH — see the README.');
    console.error(err);
    return;
  }

  const field = new TileField(scene.scene);
  const actors = new ActorField(scene.scene, field);
  let ui, input, conn;

  let lastScene = null;
  let toastUntil = 0;
  let firstScene = true;
  let sentSize = { w: 0, h: 0 };

  const send = (msg) => conn.send(msg);

  ui = new UI(send);
  input = new Input(ui, send);

  // The action camera (docs/SWORDPLAY_PLAN.md): V flips between the top-down
  // follow-cam and the over-the-shoulder duel view. One switch, three parties.
  function setView(mode) {
    scene.setMode(mode);
    input.setMode(mode);
    ui.setActionMode(mode === 'action');
  }
  input.attach({
    scene,
    actors,
    toggleView: () => setView(scene.mode === 'action' ? 'top' : 'action'),
  });
  // Esc releases the pointer but keeps the view: the next click on the world
  // re-engages mouse-look (input.js), so the default camera never dumps you
  // into the overview uninvited.

  conn = new Connection(name, {
    onMessage: (msg) => {
      switch (msg.t) {
        case 'hello': onHello(msg); break;
        case 'scene': onScene(msg); break;
        case 'chat': ui.addChat(msg); break;
        case 'panel': ui.showPanel(msg); break;
        case 'bye': ui.setStatus(msg.reason || 'disconnected', true); break;
      }
    },
    onStatus: (state) => {
      if (state === 'connected') ui.setStatus('', false);
      else ui.setStatus('reconnecting…', true);
    },
    onFatal: (reason) => {
      gate.hidden = false;
      gameEl.hidden = true;
      showGateError(reason);
    },
  });

  function onHello(msg) {
    if (msg.version !== PROTOCOL) {
      // A cached page against a newer server: fail loudly rather than render
      // something subtly wrong.
      ui.setStatus('reload — the server was updated', true);
      return;
    }
    field.setVocabulary(msg.shapes, msg.props, msg.texes);
    actors.setVocabulary(msg.shapes, msg.weapons);
    gate.hidden = true;
    gameEl.hidden = false;
    scene.resize();
    reportSize(true);
    // You arrive behind your own shoulders: the action camera is the default
    // view, V drops to the top-down overview. Pointer lock wants a user
    // gesture — the join click usually still counts; if the browser says no,
    // the first click on the world engages mouse-look instead of swinging.
    setView('action');
  }

  function onScene(msg) {
    lastScene = msg;
    if (msg.reset) actors.clear();
    field.apply(msg);
    actors.sync(msg, field.palette);
    scene.applyLighting(msg.ambient, msg.light, Math.min(msg.w || 0, msg.h || 0));
    actors.setNight(msg.ambient?.night);
    field.setNight(msg.ambient?.night);

    ui.setArea(msg.areaName || '', msg.flare);
    ui.setClaim(msg.claim);
    ui.setPrompt(msg.prompt);
    ui.setMinimap(msg.minimap || null);
    ui.setBuild(msg.build || null);
    ui.setSlide(msg.slide || null);
    if (msg.hurt) {
      ui.flashHurt(true);
      actors.flinchSelf(); // the vignette's partner: the body reacts too
    }
    if (msg.toast) {
      ui.setToast(msg.toast);
      toastUntil = performance.now() + 2600;
    }

    // Snap the camera the first time and whenever the area changes, so entering
    // somewhere doesn't fly the camera across the map.
    const self = actors.self;
    if (self) {
      if (firstScene || msg.reset) {
        self.place(self.toX, self.toZ);
        scene.follow(self.toX, self.toZ, field.heightAt(self.toX, self.toZ), true);
        firstScene = false;
      }
    }
  }

  /** reportSize tells the server how big a tile window this screen wants. The
   *  server clamps it, so asking for a wall-sized window is safe. */
  function reportSize(force = false) {
    const want = scene.tilesInView();
    if (!force && want.w === sentSize.w && want.h === sentSize.h) return;
    sentSize = want;
    send({ t: 'resize', w: want.w, h: want.h });
  }

  conn.connect();

  // Keep the connection alive through proxies that time out idle sockets.
  setInterval(() => send({ t: 'ping' }), 25000);

  let last = performance.now();
  function frame(now) {
    const dt = Math.min(0.1, (now - last) / 1000);
    last = now;
    const time = now / 1000;

    input.pump(now);
    actors.update(dt, time);
    field.update(time);

    const self = actors.self;
    if (self) scene.follow(self.x, self.z, field.heightAt(self.x, self.z));
    // Locked on: the camera steers to keep both fighters in frame.
    const lock = actors.lockName ? actors.players.get(actors.lockName) : null;
    scene.lockPoint = lock ? { x: lock.x, z: lock.z } : null;
    scene.update(dt);
    scene.render();

    ui.tickChat(now);
    if (toastUntil && now > toastUntil) { ui.setToast(''); toastUntil = 0; }
    reportSize();

    requestAnimationFrame(frame);
  }
  requestAnimationFrame(frame);

  // Expose a little state for debugging in the console; harmless, and it makes
  // "why is that tile wrong" answerable without a rebuild.
  window.durstworld = { scene, field, actors, ui, input, get lastScene() { return lastScene; } };
}
