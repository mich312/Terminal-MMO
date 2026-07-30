/* Players and creatures.
 *
 * Unlike tiles there are only ever a handful of these, so they get real objects
 * rather than instancing — which buys per-actor animation cheaply: a walk
 * cycle, a turn toward the way you're facing, and interpolation between the
 * server's discrete tile steps.
 *
 * That interpolation is the whole reason the browser feels different from the
 * terminal. The world's *cells* are a grid and always will be, but a body's
 * position in it is continuous now, and the server sends that position rather
 * than a tile — so this smooths between network samples instead of inventing a
 * walk out of tile hops. A terminal still has to snap; here we don't.
 *
 * Players are articulated rigs (rig.js) — limbs, a weapon in hand, and the
 * swordplay motions (docs/SWORDPLAY_PLAN.md): swings arrive as one-shot FX on
 * the scene message and play on whoever swung, whiffs included, so a duel is
 * readable from every camera. Creatures keep the prop-shape bodies.
 */

import * as THREE from 'three';
import { partsFor } from './props.js';
import { Rig } from './rig.js';

/* The glide.
   The server sends a body's real position now, several times a second, and it
   moves continuously between those samples rather than in tile hops (protocol
   v4). So this is plain smoothing toward the last sample: a first-order chase
   whose only job is to absorb the gap between network frames.

   It used to be much more than that — the server sent whole-tile steps at ten a
   second, and this module inferred a pace from the interval between them to
   turn those hops back into walking. None of that is needed once the thing
   arriving is already a walk. */
const FOLLOW = 16;      // how hard the drawn body chases the last sample, per second
const SNAP_TILES = 6;   // farther than this is a teleport, not a walk: cut, don't slide
const RUN_SPEED = 4.25; // tiles/sec above which the walk cycle reads as a run
const COAST_MS = 120;   // the walk cycle keeps running this long past the last motion
const BOB_HEIGHT = 0.07;

/** Facing directions, mirroring world.Dir (S, SE, E, NE, N, NW, W, SW).
 *
 *  The world's axes are x→east, z→south (field.js lays tile (x,y) at world
 *  (x,z)), and the duelist rig is built facing +Z (rig_character.js). So the
 *  rotation that points a body along a facing is atan2(dx, dz) of that
 *  facing's delta — and nothing else: these angles were previously negated,
 *  which mirrors east and west. You walked east and your character faced west,
 *  swung west, and moonwalked across the field. */
const DIR_ANGLE = [
  0,                  // S  → ( 0,  1)
  Math.PI / 4,        // SE → ( 1,  1)
  Math.PI / 2,        // E  → ( 1,  0)
  3 * Math.PI / 4,    // NE → ( 1, -1)
  Math.PI,            // N  → ( 0, -1)
  -3 * Math.PI / 4,   // NW → (-1, -1)
  -Math.PI / 2,       // W  → (-1,  0)
  -Math.PI / 4,       // SW → (-1,  1)
];

/** bodyX/bodyZ read an actor's position off the wire. Players carry a real
 *  body position (fx, fy); creatures still step whole tiles and only send the
 *  cell, so those are centred in it the way they always were. */
function bodyX(a) { return a.fx ?? a.x + 0.5; }
function bodyZ(a) { return a.fy ?? a.y + 0.5; }

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
  // Modest plates: the fighters are the picture, the names are a caption.
  sprite.scale.set(w / h * 0.34, 0.34, 1);
  sprite.renderOrder = 10;
  return sprite;
}

/** buildBody assembles a creature from the same part list the props use, so a
 *  creature on the grid and a creature baked into a tile look identical. */
function buildBody(parts, color) {
  const group = new THREE.Group();
  const c = new THREE.Color();
  const lit = [];
  for (const p of parts) {
    const material = new THREE.MeshStandardMaterial({
      color: 0xffffff,
      // Bodies are a touch smoother than the world around them, so a player
      // catches a highlight and reads as the thing that's alive on screen.
      roughness: p.rough ?? 0.62,
      metalness: p.metal ?? 0.04,
    });
    if (p.glow > 0) {
      // Lit-but-emissive, like the field's glow pools: strength honored,
      // shading kept, hue from the body color set below.
      material.emissive.copy(color);
      material.emissiveIntensity = p.glow;
    }
    if (p.double) material.side = THREE.DoubleSide;
    if (p.fixed != null) c.setHex(p.fixed);
    else c.copy(color).multiplyScalar(p.tint);
    material.color.copy(c);
    const mesh = new THREE.Mesh(p.geom, material);
    mesh.castShadow = p.glow === 0;
    mesh.receiveShadow = true;
    group.add(mesh);
    if (material.isMeshStandardMaterial) lit.push(material);
  }
  group.userData.lit = lit;
  return group;
}

class Actor {
  constructor(scene, group, rig, terrain) {
    this.scene = scene;
    this.group = group;
    this.rig = rig || null;
    this.terrain = terrain || null; // the tile field, for standing on hills
    this.x = 0; this.z = 0;         // current interpolated position
    this.toX = 0; this.toZ = 0;     // where the server says we are
    this.speed = 0;                 // observed tiles/sec, for the walk cycle
    this.lastSampleAt = 0;          // when the last position arrived, to measure it
    this.movingUntil = 0;           // the walk cycle's coast window
    this.angle = 0;
    this.targetAngle = 0;
    this.guarding = false;
    this.downed = false;
    scene.add(group);
  }

  /** moveTo takes one server position. Positions arrive continuously, so this
   *  measures how fast the body is actually travelling — which is the only way
   *  left to know whether it is walking or running, and is a better answer than
   *  the old one anyway, since it reads the truth rather than a step size. */
  moveTo(x, z) {
    if (this.toX === x && this.toZ === z) return;
    const span = Math.hypot(x - this.toX, z - this.toZ);
    if (span > SNAP_TILES) { this.place(x, z); return; } // a portal, not a stride
    const now = performance.now();
    if (this.lastSampleAt) {
      const dt = (now - this.lastSampleAt) / 1000;
      if (dt > 0.001) this.speed += (span / dt - this.speed) * 0.4;
    }
    this.lastSampleAt = now;
    this.toX = x; this.toZ = z;
  }

  place(x, z) {
    this.x = this.toX = x;
    this.z = this.toZ = z;
    this.lastSampleAt = 0;
    this.speed = 0;
  }

  update(dt, time) {
    const dx = this.toX - this.x, dz = this.toZ - this.z;
    if (Math.abs(dx) > 1e-5 || Math.abs(dz) > 1e-5) {
      const k = Math.min(1, dt * FOLLOW);
      this.x += dx * k;
      this.z += dz * k;
      this.movingUntil = time + COAST_MS / 1000;
    }
    // Coast briefly past the last motion: a frame the server skipped because
    // nothing changed shouldn't read as a stumble in the walk cycle.
    const moving = time < this.movingUntil;
    // Rigs plant their own feet; simple bodies keep the classic hop.
    const bob = moving && !this.rig ? Math.abs(Math.sin(time * 12)) * BOB_HEIGHT : 0;
    // Stand on the terrain: the interpolated position samples the same
    // heightfield the ground is displaced by, so feet track a slope mid-glide.
    const ground = this.terrain ? this.terrain.heightAt(this.x, this.z) : 0;
    this.group.position.set(this.x, ground + bob, this.z);

    // Turn the short way round toward the facing the server reported.
    let d = this.targetAngle - this.angle;
    while (d > Math.PI) d -= Math.PI * 2;
    while (d < -Math.PI) d += Math.PI * 2;
    this.angle += d * Math.min(1, dt * 12);
    this.group.rotation.y = this.angle;

    if (this.rig) {
      this.rig.pose({
        moving,
        running: this.speed > RUN_SPEED,
        guarding: this.guarding,
        downed: this.downed,
      }, time, dt);
    }
  }

  dispose() {
    this.scene.remove(this.group);
  }
}

export class ActorField {
  constructor(scene, terrain) {
    this.scene = scene;
    this.terrain = terrain || null; // the tile field, for terrain heights
    this.players = new Map();
    this.creatures = new Map();
    this.shapes = {};
    this.weaponVocab = {};
    this.self = null;
    this.lockName = null;
    this._c = new THREE.Color();
    this._localActs = new Map(); // kind → time, to skip the echo of our own swing
  }

  setVocabulary(shapes, weapons) {
    this.shapes = shapes || {};
    this.weaponVocab = weapons || {};
  }

  /** sync reconciles the live actors with one scene message. */
  sync(msg, palette) {
    this.reconcile(this.players, msg.players || [], palette, true);
    this.reconcile(this.creatures, msg.creatures || [], palette, false);
    const now = performance.now() / 1000;
    for (const fx of msg.fx || []) this.playFX(fx, now);
    if (this.lockName && !this.players.get(this.lockName)) this.setLock(null);
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
        // Spawned mid-night: light it now rather than at the next change.
        for (const m of actor.group.userData.lit || []) {
          m.emissive.copy(m.color);
          m.emissiveIntensity = 0.42 * (this._night || 0) * (isPlayer ? 1 : 0.5);
        }
        actor.place(bodyX(a), bodyZ(a));
      } else {
        actor.moveTo(bodyX(a), bodyZ(a));
      }
      // A player carries a continuous heading; a creature still only has one of
      // eight, so fall back to the table for them.
      actor.targetAngle = a.ang ?? DIR_ANGLE[a.f] ?? 0;
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
    let group, rig = null;
    if (isPlayer) {
      rig = new Rig(color, a.a);
      group = rig.group;
      group.userData.lit = rig.lit;
      const label = nameSprite(a.n, '#' + color.getHexString());
      label.position.set(0, 1.34, 0);
      group.add(label);
      group.userData.label = label;
    } else {
      const parts = a.k && this.shapes[a.k] ? partsFor(a.k, this.shapes[a.k]) : null;
      if (!parts) return null; // an unknown species draws nothing rather than a
      // mystery box wandering the fields
      group = buildBody(parts, color);
    }
    return new Actor(this.scene, group, rig, this.terrain);
  }

  /** stylePlayer reflects live state a player's body should show: the wielded
   *  arm, a raised guard, knocked out, or hurt and not yet healed. */
  stylePlayer(actor, a) {
    actor.rig?.setWeapon(this.weaponVocab[a.w] || '');
    actor.guarding = !!a.g;
    const downed = !!a.down;
    if (actor.downed !== downed) {
      actor.downed = downed;
      // The rig plays the crumple; the fade makes "out cold" unmistakable.
      actor.group.traverse((o) => {
        if (o.isMesh && !o.isSprite) {
          o.material.opacity = downed ? 0.6 : 1;
          o.material.transparent = downed;
        }
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

  /** playFX routes one combat motion to its actor. Our own verbs already
   *  played the instant we pressed them (localAct), so their echo — the same
   *  motion arriving back off the wire moments later — is skipped. */
  playFX(fx, now) {
    const actor = this.players.get(fx.n);
    if (!actor) return;
    if (actor === this.self) {
      const at = this._localActs.get(fx.k);
      if (at != null && now - at < 0.6) return;
    }
    actor.rig?.play(fx.k, now);
    if (fx.k === 'parry') {
      // The parried attacker reels.
      this.players.get(fx.t)?.rig?.play('flinch', now);
    }
  }

  /** localAct plays one of our own verbs immediately — the client animates
   *  hopefully, the server referees — and remembers it to skip the echo. */
  localAct(kind) {
    const now = performance.now() / 1000;
    this._localActs.set(kind, now);
    this.self?.rig?.play(kind, now);
  }

  /** flinchSelf is the on-hit body reaction (the vignette's partner). */
  flinchSelf() {
    this.self?.rig?.play('flinch', performance.now() / 1000);
  }

  /** setLock marks the locked-on target with a ring at its feet. */
  setLock(name) {
    if (this.lockName === name) return;
    const prev = this.players.get(this.lockName);
    if (prev && prev.lockRing) {
      prev.group.remove(prev.lockRing);
      prev.lockRing = null;
    }
    this.lockName = name;
    const next = this.players.get(name);
    if (next) {
      const ring = new THREE.Mesh(
        new THREE.RingGeometry(0.42, 0.5, 24),
        new THREE.MeshBasicMaterial({ color: 0xe8b34c, transparent: true, opacity: 0.85 }),
      );
      ring.rotation.x = -Math.PI / 2;
      ring.position.y = 0.03;
      next.group.add(ring);
      next.lockRing = ring;
    }
  }

  /** nearestTarget picks the closest other player, nudged toward whoever is in
   *  front of the camera when two are equally close — the lock-on's opening
   *  bid. Never nobody-because-they're-behind-you: the camera swings to them.
   */
  nearestTarget(px, pz, fwdX, fwdZ, maxDist = 14) {
    let best = null, bestScore = maxDist;
    for (const [name, a] of this.players) {
      if (a === this.self || a.downed) continue;
      const dx = a.x - px, dz = a.z - pz;
      const d = Math.hypot(dx, dz);
      if (d >= maxDist || d < 0.01) continue;
      const behind = (dx * fwdX + dz * fwdZ) < 0 ? 1.5 : 0;
      if (d + behind >= bestScore) continue;
      best = name;
      bestScore = d + behind;
    }
    return best;
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

  /** setNight keeps bodies readable after dark.
   *
   *  internal/ui/atmosphere.go is explicit about this: the day/night wash is
   *  applied to tiles but "player glyphs are left untouched so avatars stay
   *  readable at night". The terminal gets that by simply not tinting them; in
   *  a lit 3D scene the equivalent is a floor of self-illumination, so a body
   *  never disappears into an unlit field. Creatures get it too, at half — you
   *  should be able to make out a deer at midnight, but not mistake it for
   *  midday.
   */
  setNight(n) {
    const night = Math.max(0, Math.min(1, n || 0));
    if (this._night === night) return;
    this._night = night;
    const apply = (actor, scale) => {
      for (const m of actor.group.userData.lit || []) {
        m.emissive.copy(m.color);
        m.emissiveIntensity = 0.42 * night * scale;
      }
    };
    for (const a of this.players.values()) apply(a, 1);
    for (const a of this.creatures.values()) apply(a, 0.5);
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
    this.lockName = null;
  }
}
