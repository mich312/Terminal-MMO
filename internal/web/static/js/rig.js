/* The hero rig (docs/SWORDPLAY_PLAN.md).
 *
 * An articulated armored duelist, assembled and animated by three focused
 * modules — the body (rig_character.js), the arms catalog (rig_weapons.js)
 * and the combat motion set (rig_animation.js) — all procedural: geometry and
 * curves in code, no asset files, nothing to embed beyond the source. This
 * file is the thin skeleton contract between them: the joint hierarchy, the
 * verb state machine, and the API actors.js drives.
 *
 * One rig serves everyone and both cameras: the top-down view gets the same
 * walking, swinging, guarding body the action camera stares at.
 */

import * as THREE from 'three';
import { buildDuelist } from './rig_character.js';
import { buildWeapon, weaponAccent } from './rig_weapons.js';
import { initAnim, poseRig } from './rig_animation.js';

/** weaponMesh builds a held arm by silhouette class (the hello's weapons
 *  vocabulary). Grip at the origin, business end along -Y at rest. */
export function weaponMesh(cls) {
  return buildWeapon(cls, THREE);
}

export class Rig {
  constructor(color, accessory) {
    this.group = new THREE.Group(); // facing/position applied here by Actor
    this.root = new THREE.Group();  // pose engine's own axis (KO, dodge tuck)
    this.group.add(this.root);
    this.lit = [];

    // The body: builds and parents every joint in the contract — torso, head,
    // armL/R, foreL/R, hand, legL/R, cloak — and fills this.lit.
    buildDuelist(this, color, accessory, THREE);

    this.weaponCls = '';
    this.walkPhase = 0;
    this.moveW = 0;   // smoothed "am I moving" weight
    this.guardW = 0;  // guard stance blend, eased toward the server's state
    this.downedW = 0; // knock-out blend
    this.act = null;  // one-shot motion: {kind, start, mirror}
    this.flinchUntil = 0;
    this.swingAlt = false;

    // The animator's own state: smoothed weights and the swing trail.
    initAnim(this);
  }

  setWeapon(cls) {
    if (cls === this.weaponCls) return;
    this.weaponCls = cls;
    if (this.held) this.hand.remove(this.held);
    this.held = weaponMesh(cls);
    if (this.held) this.hand.add(this.held);
    this.trailColor = weaponAccent(cls);
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

  /** pose computes the whole skeleton for this frame — stateless given
   *  (state, time) plus the smoothed weights, so there is nothing to get
   *  stuck. The heavy lifting lives in rig_animation.js. */
  pose(state, time, dt) {
    poseRig(this, state, time, dt, THREE);
  }
}
