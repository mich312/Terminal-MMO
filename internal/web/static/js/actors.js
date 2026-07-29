/* Players and creatures.
 *
 * Unlike tiles there are only ever a handful of these, so they get real objects
 * rather than instancing — which buys per-actor animation cheaply: a walk bob, a
 * turn toward the way you're facing, and interpolation between the server's
 * discrete tile steps.
 *
 * That interpolation is the whole reason the browser feels different from the
 * terminal. The world is a grid and always was: the server says "anna is now on
 * tile (12, 7)". A terminal has to snap. Here we glide, so the same 10-steps-
 * per-second movement reads as walking rather than teleporting.
 */

import * as THREE from 'three';
import { partsFor } from './props.js';

const STEP_MS = 110;   // how long an actor takes to glide one tile
const BOB_HEIGHT = 0.07;

/** Facing directions, mirroring world.Dir (S, SE, E, NE, N, NW, W, SW). */
const DIR_ANGLE = [
  0,                  // S
  -Math.PI / 4,       // SE
  -Math.PI / 2,       // E
  -3 * Math.PI / 4,   // NE
  Math.PI,            // N
  3 * Math.PI / 4,    // NW
  Math.PI / 2,        // W
  Math.PI / 4,        // SW
];

/** nameSprite renders a name into a canvas texture the camera always faces. */
function nameSprite(text, hex) {
  const pad = 8, font = '600 34px ui-monospace, Menlo, Consolas, monospace';
  const measure = document.createElement('canvas').getContext('2d');
  measure.font = font;
  const w = Math.ceil(measure.measureText(text).width) + pad * 2;
  const h = 48;
  const canvas = document.createElement('canvas');
  canvas.width = w;
  canvas.height = h;
  const ctx = canvas.getContext('2d');
  ctx.font = font;
  ctx.textBaseline = 'middle';
  // A dark plate behind the text keeps names legible over snow and over water.
  ctx.fillStyle = 'rgba(8, 11, 15, 0.72)';
  ctx.beginPath();
  ctx.roundRect(0, 4, w, h - 8, 8);
  ctx.fill();
  ctx.fillStyle = hex || '#e8ecf1';
  ctx.fillText(text, pad, h / 2 + 1);

  const tex = new THREE.CanvasTexture(canvas);
  tex.colorSpace = THREE.SRGBColorSpace;
  const sprite = new THREE.Sprite(new THREE.SpriteMaterial({
    map: tex, transparent: true, depthWrite: false,
  }));
  sprite.scale.set(w / h * 0.5, 0.5, 1);
  sprite.renderOrder = 10;
  return sprite;
}

/** buildBody assembles an avatar from the same part list the props use, so a
 *  creature on the grid and a creature baked into a tile look identical. */
function buildBody(parts, color) {
  const group = new THREE.Group();
  const c = new THREE.Color();
  for (const p of parts) {
    const material = p.glow > 0
      ? new THREE.MeshBasicMaterial({ color: 0xffffff })
      : new THREE.MeshLambertMaterial({ color: 0xffffff });
    if (p.double) material.side = THREE.DoubleSide;
    if (p.fixed != null) c.setHex(p.fixed);
    else c.copy(color).multiplyScalar(p.tint);
    material.color.copy(c);
    group.add(new THREE.Mesh(p.geom, material));
  }
  return group;
}

/* The player avatar. Deliberately not a creature shape: a person needs to read
   as a person from above, which means shoulders and a head, and a clear front
   so facing is visible from a 3/4 camera. */
function playerParts() {
  const parts = [];
  const body = new THREE.CapsuleGeometry(0.2, 0.34, 4, 8);
  body.translate(0, 0.42, 0);
  parts.push({ geom: body, tint: 1, fixed: null, glow: 0 });
  const head = new THREE.SphereGeometry(0.16, 10, 8);
  head.translate(0, 0.84, 0);
  parts.push({ geom: head, tint: 1.15, fixed: null, glow: 0 });
  // A small nose-like wedge on the front face: the cheapest possible "which way
  // am I pointing", and it survives being 30 pixels tall on screen.
  const snout = new THREE.BoxGeometry(0.1, 0.08, 0.12);
  snout.translate(0, 0.84, 0.17);
  parts.push({ geom: snout, tint: 0.75, fixed: null, glow: 0 });
  return parts;
}

const PLAYER_PARTS = playerParts();

/* A worn hat, drawn when the player has an accessory. Which hat you found is
   in the compendium; here every hat is a hat — the color carries the rest. */
function hatParts() {
  const brim = new THREE.CylinderGeometry(0.23, 0.25, 0.04, 10);
  brim.translate(0, 0.97, 0);
  const crown = new THREE.CylinderGeometry(0.13, 0.15, 0.14, 10);
  crown.translate(0, 1.05, 0);
  return [
    { geom: brim, tint: 0.9, fixed: null, glow: 0.2 },
    { geom: crown, tint: 1.3, fixed: null, glow: 0.2 },
  ];
}

const HAT_PARTS = hatParts();

class Actor {
  constructor(scene, group) {
    this.scene = scene;
    this.group = group;
    this.x = 0; this.z = 0;         // current interpolated position
    this.fromX = 0; this.fromZ = 0; // where the glide started
    this.toX = 0; this.toZ = 0;     // where the server says we are
    this.t = 1;                     // glide progress, 0..1
    this.angle = 0;
    this.targetAngle = 0;
    scene.add(group);
  }

  moveTo(x, z) {
    if (this.toX === x && this.toZ === z) return;
    this.fromX = this.x; this.fromZ = this.z;
    this.toX = x; this.toZ = z;
    this.t = 0;
  }

  place(x, z) {
    this.x = this.fromX = this.toX = x;
    this.z = this.fromZ = this.toZ = z;
    this.t = 1;
  }

  update(dt, time) {
    if (this.t < 1) {
      this.t = Math.min(1, this.t + dt * 1000 / STEP_MS);
      // Ease-out: fast off the mark, settling onto the tile.
      const e = 1 - Math.pow(1 - this.t, 3);
      this.x = this.fromX + (this.toX - this.fromX) * e;
      this.z = this.fromZ + (this.toZ - this.fromZ) * e;
    }
    // A short bob while moving sells the step; standing still, it rests.
    const moving = this.t < 1;
    const bob = moving ? Math.abs(Math.sin(time * 12)) * BOB_HEIGHT : 0;
    this.group.position.set(this.x, bob, this.z);

    // Turn the short way round toward the facing the server reported.
    let d = this.targetAngle - this.angle;
    while (d > Math.PI) d -= Math.PI * 2;
    while (d < -Math.PI) d += Math.PI * 2;
    this.angle += d * Math.min(1, dt * 12);
    this.group.rotation.y = this.angle;
  }

  dispose() {
    this.scene.remove(this.group);
  }
}

export class ActorField {
  constructor(scene) {
    this.scene = scene;
    this.players = new Map();
    this.creatures = new Map();
    this.shapes = {};
    this.self = null;
    this._c = new THREE.Color();
  }

  setVocabulary(shapes) { this.shapes = shapes || {}; }

  /** sync reconciles the live actors with one scene message. */
  sync(msg, palette) {
    this.reconcile(this.players, msg.players || [], palette, true);
    this.reconcile(this.creatures, msg.creatures || [], palette, false);
  }

  reconcile(map, list, palette, isPlayer) {
    const seen = new Set();
    for (const a of list) {
      const id = a.n || '';
      seen.add(id);
      let actor = map.get(id);
      const color = palette[a.c] || new THREE.Color(0xcccccc);
      if (!actor) {
        actor = this.spawn(a, color, isPlayer);
        if (!actor) continue;
        map.set(id, actor);
        actor.place(a.x + 0.5, a.y + 0.5);
      } else {
        actor.moveTo(a.x + 0.5, a.y + 0.5);
      }
      actor.targetAngle = DIR_ANGLE[a.f] ?? 0;
      if (isPlayer) {
        this.stylePlayer(actor, a);
        if (a.me) this.self = actor;
      }
    }
    for (const [id, actor] of map) {
      if (!seen.has(id)) {
        actor.dispose();
        map.delete(id);
        if (this.self === actor) this.self = null;
      }
    }
  }

  spawn(a, color, isPlayer) {
    let group;
    if (isPlayer) {
      group = buildBody(PLAYER_PARTS, color);
      if (a.a) group.add(...buildBody(HAT_PARTS, color).children);
      const label = nameSprite(a.n, '#' + color.getHexString());
      label.position.set(0, 1.45, 0);
      group.add(label);
      group.userData.label = label;
    } else {
      const parts = a.k && this.shapes[a.k] ? partsFor(a.k, this.shapes[a.k]) : null;
      if (!parts) return null; // an unknown species draws nothing rather than a
      // mystery box wandering the fields
      group = buildBody(parts, color);
    }
    return new Actor(this.scene, group);
  }

  /** stylePlayer reflects live state a player's body should show: knocked out,
   *  or hurt and not yet healed. */
  stylePlayer(actor, a) {
    const downed = !!a.down;
    if (actor.downed !== downed) {
      actor.downed = downed;
      // Knocked out: the body tips over. It's unmistakable from any angle and
      // costs nothing.
      actor.group.rotation.x = downed ? Math.PI / 2.2 : 0;
      actor.group.traverse((o) => {
        if (o.isMesh) o.material.opacity = downed ? 0.6 : 1;
        if (o.isMesh) o.material.transparent = downed;
      });
    }
    if (a.hp != null && a.mhp && a.hp < a.mhp && !actor.hpBar) {
      actor.hpBar = this.makeHPBar();
      actor.group.add(actor.hpBar);
    }
    if (actor.hpBar) {
      const frac = a.mhp ? Math.max(0, a.hp / a.mhp) : 1;
      actor.hpBar.visible = frac < 1;
      // Drain from the right: shrink the bar and shift it left by half the loss
      // so its left edge stays put.
      actor.hpBar.scale.x = Math.max(0.001, 0.5 * frac);
      actor.hpBar.position.x = -(1 - frac) * 0.25;
    }
  }

  // A sprite, not a quad: it faces the camera on its own, so it stays readable
  // whether the body it belongs to has turned toward you or away.
  makeHPBar() {
    const bar = new THREE.Sprite(new THREE.SpriteMaterial({
      color: 0xe25555, depthTest: false, transparent: true,
    }));
    bar.scale.set(0.5, 0.07, 1);
    bar.position.set(0, 1.25, 0);
    bar.renderOrder = 11;
    bar.visible = false;
    return bar;
  }

  /** update advances every actor. Labels and bars are sprites, so they keep
   *  themselves pointed at the camera. */
  update(dt, time) {
    for (const a of this.players.values()) a.update(dt, time);
    for (const a of this.creatures.values()) a.update(dt, time);
  }

  clear() {
    for (const a of this.players.values()) a.dispose();
    for (const a of this.creatures.values()) a.dispose();
    this.players.clear();
    this.creatures.clear();
    this.self = null;
  }
}
