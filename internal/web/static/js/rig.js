/* The hero rig (docs/SWORDPLAY_PLAN.md).
 *
 * An articulated figure — torso, head, two arms, two legs, a weapon in the
 * right hand — built from the same primitive vocabulary as props.js and
 * animated procedurally: every pose is a handful of sine/ease curves on joint
 * rotations, the same school as the wind shader. No asset files, no skeletal
 * clips, nothing to embed; a swing is thirty lines of math.
 *
 * One rig serves everyone and both cameras: the top-down view gets the same
 * walking, swinging, guarding body the action camera stares at. The capsule
 * avatar it replaces read fine from 52° above, but "the player is a 3D model
 * in front of the camera" is a renderer demand — a protagonist needs limbs.
 */

import * as THREE from 'three';

const STEEL = 0xd8dde2;
const HAFT = 0x7a5a38;
const GUARD_GOLD = 0x8a6d3b;
const GLOW = 0xffe9a8;

/* Joint pivots, in the figure's local space. The head center lands at the same
   height (0.85) as the old capsule avatar, so hats keep fitting. */
const HIP_Y = 0.42;
const SHOULDER_Y = 0.72;
const SHOULDER_X = 0.24; // clear of the torso, so arms read as arms
const HEAD_PIVOT_Y = 0.74;

function std(color, rough = 0.62, metal = 0.04) {
  return new THREE.MeshStandardMaterial({ color, roughness: rough, metalness: metal });
}

/** limb makes a pivoting group holding one box mesh that hangs below the
 *  pivot, so rotating the group swings the limb like a joint. */
function limb(w, len, d, material) {
  const g = new THREE.Group();
  const geom = new THREE.BoxGeometry(w, len, d);
  geom.translate(0, -len / 2, 0);
  const mesh = new THREE.Mesh(geom, material);
  mesh.castShadow = true;
  mesh.receiveShadow = true;
  g.add(mesh);
  return g;
}

/** weaponMesh builds a held arm by silhouette class (the hello's weapons
 *  vocabulary). Local space: the grip is at the origin, the business end
 *  extends -Y — hanging along a lowered arm, arcing with a swing. */
export function weaponMesh(cls) {
  if (!cls) return null;
  const legend = cls.endsWith('_legend');
  const base = legend ? cls.slice(0, -'_legend'.length) : cls;
  const g = new THREE.Group();
  const steel = legend
    ? new THREE.MeshStandardMaterial({
      color: STEEL, roughness: 0.3, metalness: 0.5,
      emissive: GLOW, emissiveIntensity: 0.55,
    })
    : std(STEEL, 0.35, 0.7);
  const wood = std(HAFT, 0.7, 0);
  const add = (geom, material) => {
    const mesh = new THREE.Mesh(geom, material);
    mesh.castShadow = true;
    g.add(mesh);
    return mesh;
  };

  switch (base) {
    case 'blade':
    case 'blade_s': {
      const len = base === 'blade' ? 0.52 : 0.3;
      add(new THREE.CylinderGeometry(0.02, 0.022, 0.12, 6), wood);
      const cross = new THREE.BoxGeometry(0.14, 0.025, 0.035);
      cross.translate(0, -0.07, 0);
      add(cross, std(GUARD_GOLD, 0.45, 0.5));
      const blade = new THREE.BoxGeometry(0.05, len, 0.015);
      blade.translate(0, -0.08 - len / 2, 0);
      add(blade, steel);
      break;
    }
    case 'polearm': {
      const shaft = new THREE.CylinderGeometry(0.017, 0.017, 1.05, 6);
      shaft.translate(0, 0.2, 0); // held mid-haft, tip riding high
      add(shaft, wood);
      const tip = new THREE.ConeGeometry(0.035, 0.16, 6);
      tip.translate(0, 0.8, 0);
      add(tip, steel);
      break;
    }
    case 'bow': {
      const arc = new THREE.TorusGeometry(0.3, 0.016, 6, 16, Math.PI * 0.92);
      arc.rotateZ(Math.PI / 2 - Math.PI * 0.46); // arc opening toward the string
      add(arc, wood);
      const string = new THREE.BoxGeometry(0.006, 0.55, 0.006);
      string.translate(0.09, 0, 0);
      add(string, std(0xcfd6dd, 0.8, 0));
      if (legend) g.children[0].material = steel;
      break;
    }
    case 'sling': {
      const strap = new THREE.BoxGeometry(0.03, 0.34, 0.02);
      strap.translate(0, -0.16, 0);
      add(strap, std(0x6b4a2f, 0.85, 0));
      const pouch = new THREE.SphereGeometry(0.045, 6, 5);
      pouch.translate(0, -0.33, 0);
      add(pouch, wood);
      break;
    }
    default:
      return null;
  }
  return g;
}

/** Rig assembles the articulated body. `color` is the player's own; parts are
 *  tinted from it exactly as the old avatar was, so a player keeps their look. */
export class Rig {
  constructor(color, accessory) {
    this.group = new THREE.Group(); // facing/position applied here by Actor
    this.root = new THREE.Group();  // pose engine's own axis (KO lie-down, rolls)
    this.group.add(this.root);
    this.lit = [];

    const c = new THREE.Color();
    const tinted = (tint) => {
      c.copy(color).multiplyScalar(tint);
      const m = std(c.clone());
      this.lit.push(m);
      return m;
    };

    // Torso, pivoting at the hips so heavy swings can put the back into it.
    this.torso = new THREE.Group();
    this.torso.position.y = HIP_Y;
    const chest = new THREE.BoxGeometry(0.34, 0.34, 0.2);
    chest.translate(0, 0.17, 0);
    const chestMesh = new THREE.Mesh(chest, tinted(1));
    chestMesh.castShadow = true;
    chestMesh.receiveShadow = true;
    this.torso.add(chestMesh);
    this.root.add(this.torso);

    // Head — a child of the torso, with the old avatar's face heights.
    this.head = new THREE.Group();
    this.head.position.y = HEAD_PIVOT_Y - HIP_Y;
    const skull = new THREE.SphereGeometry(0.15, 10, 8);
    skull.translate(0, 0.11, 0);
    const skullMesh = new THREE.Mesh(skull, tinted(1.15));
    skullMesh.castShadow = true;
    this.head.add(skullMesh);
    const snout = new THREE.BoxGeometry(0.1, 0.08, 0.12);
    snout.translate(0, 0.11, 0.15);
    this.head.add(new THREE.Mesh(snout, tinted(0.75)));
    if (accessory) {
      // Hat geometry carries world heights (brim at 0.97); re-base it onto the
      // head pivot so it rides a nod or a knock-out.
      const hat = new THREE.Group();
      hat.position.y = -HEAD_PIVOT_Y;
      const brim = new THREE.CylinderGeometry(0.23, 0.25, 0.04, 10);
      brim.translate(0, 0.97, 0);
      hat.add(new THREE.Mesh(brim, tinted(0.9)));
      const crown = new THREE.CylinderGeometry(0.13, 0.15, 0.14, 10);
      crown.translate(0, 1.05, 0);
      hat.add(new THREE.Mesh(crown, tinted(1.3)));
      this.head.add(hat);
    }
    this.torso.add(this.head);

    // Arms hang from the shoulders; the right hand carries the weapon.
    this.armL = limb(0.1, 0.34, 0.12, tinted(0.78));
    this.armL.position.set(-SHOULDER_X, SHOULDER_Y - HIP_Y, 0);
    this.torso.add(this.armL);
    this.armR = limb(0.1, 0.34, 0.12, tinted(0.78));
    this.armR.position.set(SHOULDER_X, SHOULDER_Y - HIP_Y, 0);
    this.torso.add(this.armR);
    this.hand = new THREE.Group();
    this.hand.position.y = -0.34;
    this.armR.add(this.hand);

    // Legs pivot at the hips.
    this.legL = limb(0.13, HIP_Y, 0.15, tinted(0.8));
    this.legL.position.set(-0.09, HIP_Y, 0);
    this.root.add(this.legL);
    this.legR = limb(0.13, HIP_Y, 0.15, tinted(0.8));
    this.legR.position.set(0.09, HIP_Y, 0);
    this.root.add(this.legR);

    this.weaponCls = '';
    this.walkPhase = 0;
    this.moveW = 0;   // smoothed "am I moving" weight
    this.guardW = 0;  // guard stance blend, eased toward the server's state
    this.downedW = 0; // knock-out blend
    this.act = null;  // one-shot motion: {kind, start, mirror}
    this.flinchUntil = 0;
    this.swingAlt = false;
  }

  setWeapon(cls) {
    if (cls === this.weaponCls) return;
    this.weaponCls = cls;
    if (this.held) this.hand.remove(this.held);
    this.held = weaponMesh(cls);
    if (this.held) this.hand.add(this.held);
  }

  /** play starts a one-shot motion: 'fast', 'strong', 'dodge', 'parry',
   *  'flinch'. Swings alternate sides so a flurry reads as a combo. */
  play(kind, now) {
    if (kind === 'flinch') {
      this.flinchUntil = now + 0.28;
      return;
    }
    this.swingAlt = kind === 'fast' ? !this.swingAlt : this.swingAlt;
    this.act = { kind, start: now, mirror: this.swingAlt };
  }

  /** pose computes the whole skeleton for this frame from scratch — stateless
   *  given (state, time), so there is nothing to get stuck. */
  pose(state, time, dt) {
    const { moving, running, guarding, downed } = state;

    this.guardW += ((guarding ? 1 : 0) - this.guardW) * Math.min(1, dt * 14);
    this.downedW += ((downed ? 1 : 0) - this.downedW) * Math.min(1, dt * 6);
    if (moving) this.walkPhase += dt * (running ? 13 : 9);

    // The walk: legs scissor, arms counter-swing, all scaled by a smoothed
    // "am I moving" weight so stopping settles instead of freezing.
    this.moveW += ((moving ? 1 : 0) - this.moveW) * Math.min(1, dt * 10);
    const sw = Math.sin(this.walkPhase) * (running ? 0.85 : 0.6) * this.moveW;
    let legL = sw, legR = -sw;
    let armLX = -sw * 0.5, armRX = sw * 0.5;
    let armLZ = 0.06, armRZ = -0.06;
    let torsoX = 0.04 * this.moveW, torsoY = 0;
    let headX = 0;

    // Idle breathing, fading out while anything else is going on.
    const idle = (1 - this.moveW) * (1 - this.guardW);
    armLX += Math.sin(time * 1.7) * 0.05 * idle;
    armRX += Math.sin(time * 1.7 + 0.4) * 0.05 * idle;

    // Guard stance: blade up and across, off-hand rising, a slight crouch.
    if (this.guardW > 0.01) {
      armRX = mix(armRX, -1.9, this.guardW);
      armRZ = mix(armRZ, -0.5, this.guardW);
      armLX = mix(armLX, -0.7, this.guardW);
      torsoX = mix(torsoX, 0.1, this.guardW);
    }

    // One-shot motions override the sword arm (and put the torso into the
    // heavy ones). Envelopes are plain eases over the motion's length.
    if (this.act) {
      const dur = { fast: 0.28, strong: 0.6, dodge: 0.38, parry: 0.22 }[this.act.kind] ?? 0.3;
      const t = (time - this.act.start) / dur;
      if (t >= 1) {
        this.act = null;
      } else {
        const m = this.act.mirror ? -1 : 1;
        switch (this.act.kind) {
          case 'fast': {
            // Up and back, then a diagonal cut through; recover.
            const e = t < 0.35 ? easeOut(t / 0.35) : 1 - easeIn((t - 0.35) / 0.65) * 0.9;
            armRX = -2.4 * e + 0.9 * Math.max(0, t - 0.35) / 0.65 * 2;
            armRZ = m * 0.7 * Math.sin(t * Math.PI);
            torsoY = m * 0.25 * Math.sin(t * Math.PI);
            break;
          }
          case 'strong': {
            // A long wind-up — the telegraph is the point — then the drop.
            if (t < 0.55) {
              const e = easeOut(t / 0.55);
              armRX = -2.9 * e;
              torsoY = -0.5 * e;
              torsoX = -0.12 * e;
            } else {
              const e = easeIn((t - 0.55) / 0.45);
              armRX = -2.9 + 3.8 * e;
              torsoY = -0.5 + 0.9 * e;
              torsoX = -0.12 + 0.4 * e;
            }
            break;
          }
          case 'dodge': {
            // A tucked lunge: lean hard into the roll, legs gathered.
            const arc = Math.sin(t * Math.PI);
            torsoX = 1.1 * arc;
            legL = legR = 0.9 * arc;
            armLX = armRX = 0.5 * arc;
            this.root.position.y = -0.14 * arc;
            break;
          }
          case 'parry': {
            // The blade flicks up and out — the spark's home.
            const arc = Math.sin(t * Math.PI);
            armRX = -1.9 - 0.6 * arc;
            armRZ = -0.5 - 0.5 * arc;
            break;
          }
        }
      }
    }
    if (!this.act || this.act.kind !== 'dodge') this.root.position.y = 0;

    // Taking a hit: a sharp backward flinch that decays.
    if (time < this.flinchUntil) {
      const f = (this.flinchUntil - time) / 0.28;
      torsoX -= 0.35 * f;
      headX -= 0.3 * f;
    }

    // The knock-out: the whole figure eases over sideways, arms gone slack.
    if (this.downedW > 0.01) {
      const d = this.downedW;
      this.root.rotation.x = mix(0, Math.PI / 2.15, d);
      this.root.position.y = mix(this.root.position.y, 0.12, d);
      armLX = mix(armLX, 0.4, d);
      armRX = mix(armRX, 0.3, d);
      legL = mix(legL, 0.15, d);
      legR = mix(legR, -0.1, d);
    } else {
      this.root.rotation.x = 0;
    }

    this.legL.rotation.x = legL;
    this.legR.rotation.x = legR;
    this.armL.rotation.x = armLX;
    this.armL.rotation.z = armLZ;
    this.armR.rotation.x = armRX;
    this.armR.rotation.z = armRZ;
    this.torso.rotation.x = torsoX;
    this.torso.rotation.y = torsoY;
    this.head.rotation.x = headX;
  }
}

function mix(a, b, t) { return a + (b - a) * t; }
function easeOut(t) { return 1 - Math.pow(1 - clamp01(t), 3); }
function easeIn(t) { return Math.pow(clamp01(t), 2.2); }
function clamp01(t) { return t < 0 ? 0 : t > 1 ? 1 : t; }
