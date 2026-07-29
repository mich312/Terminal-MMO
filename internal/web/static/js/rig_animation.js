/* animation.js — the duelist's motion set (Durst World hero rig v2).
 *
 * Two exported entry points:
 *   initAnim(rig)                    — preallocate all animation state + the
 *                                      swing-trail ribbon (child of rig.hand)
 *   poseRig(rig, state, time, dt, THREE) — pose the whole skeleton, every frame
 *
 * School: pure functions of (state, time) shaped by a handful of smoothed
 * weights. Every joint channel is computed as a TARGET each frame (walk/idle/
 * guard base, act override mixed in by an envelope, downed sprawl mixed last),
 * then run through one critically-fast low-pass so any interruption — an act
 * replaced mid-flight, guard dropped mid-swing — blends over 2-3 frames
 * instead of popping. The cloak is a real spring (position + velocity) so it
 * carries inertia and can be kicked.
 *
 * Sign conventions (per CONTRACT.md, matching rig.js exactly):
 *   figure faces +Z at rest;
 *   limbs hang -Y from their pivot: rotation.x NEGATIVE swings the limb
 *   forward/up; for the torso/root (mass above the pivot) rotation.x POSITIVE
 *   leans forward.
 *
 * Rules kept: nothing outside rig.* is ever touched; zero allocation inside
 * poseRig; every stateful number is initialized in initAnim (no NaN paths —
 * dt is sanitized, act progress is clamped, weights start at 0).
 */

/* ---------------------------------------------------------------- helpers */

function clamp01(t) { return t < 0 ? 0 : t > 1 ? 1 : t; }
function mix(a, b, t) { return a + (b - a) * t; }
/** settle-out: fast start, soft arrival (anticipation snaps, recovery eases) */
function easeOut(t) { t = clamp01(t); const u = 1 - t; return 1 - u * u * u; }
/** strike: slow->fast, arriving at maximum velocity — the hard stop at the
 *  end of this curve IS the contact frame. */
function easeIn(t) { t = clamp01(t); return t * t * t; }
/** smooth release for recoveries. */
function sstep(t) { t = clamp01(t); return t * t * (3 - 2 * t); }

const DUR = { fast: 0.28, strong: 0.6, dodge: 0.38, parry: 0.22 };

/* ------------------------------------------------------------------ init */

/** initAnim wires the per-rig animation state and builds the swing trail.
 *  Idempotent enough to survive being called on a fresh rig; everything
 *  poseRig will ever read or write is created here. */
export function initAnim(rig) {
  // Weights the old constructor already owns — make them numbers no matter what.
  rig.walkPhase = rig.walkPhase || 0;
  rig.moveW = rig.moveW || 0;
  rig.guardW = rig.guardW || 0;
  rig.downedW = rig.downedW || 0;

  const A = rig._anim = {
    runW: 0,          // running blend (lean, stride, arm pump)
    j: {              // smoothed joint channel buffer (the low-pass state)
      rootRX: 0, rootRZ: 0, rootPY: 0,
      torsoRX: 0, torsoRY: 0, torsoPY: 0,
      headRX: 0, headRY: 0,
      armLRX: 0, armLRZ: 0, armRRX: 0, armRRZ: 0,
      foreLRX: 0, foreRRX: 0,
      handRX: 0, handRZ: 0,
      legLRX: 0, legRRX: 0,
    },
    cloakX: 0.08,     // cloak spring position (rotation.x) + velocity
    cloakV: 0,
    lastAct: null,    // edge detection for one-shot impulses (dodge kick)
    torsoBaseY: rig.torso.position.y,
    rootBaseY: rig.root.position.y,
    handBaseY: rig.hand.position.y,
    trailMesh: null,
    trailMat: null,
  };

  /* Swing trail: a static fan of triangles in the hand's local Y/Z plane
   * (the plane the blade sweeps through when the shoulder swings), additive
   * and vertex-faded so it reads as a motion streak. Built exactly once; the
   * geometry work happens on the first poseRig call because THREE arrives
   * there as a parameter (this module imports nothing). Per-frame cost after
   * that is one opacity write and a visibility flag. */
  A.trailPending = true;

  return rig;
}

function buildTrail(rig, A, THREE) {
  A.trailPending = false;
  const NSEG = 6;          // six triangles — cheap and plenty at this scale
  const R0 = 0.30;         // fan radius near the leading edge…
  const R1 = 0.62;         // …growing along the trail (matches blade reach)
  const A0 = -0.18;        // start just ahead of the blade line (-Y)
  const A1 = 1.30;         // sweep back through the cut plane

  const verts = new Float32Array(NSEG * 3 * 3);
  const cols = new Float32Array(NSEG * 3 * 3);
  const ox = 0, oy = -0.10, oz = 0; // fan origin: just below the grip
  let v = 0, c = 0;
  for (let i = 0; i < NSEG; i++) {
    const a0 = A0 + (A1 - A0) * (i / NSEG);
    const a1 = A0 + (A1 - A0) * ((i + 1) / NSEG);
    const r0 = R0 + (R1 - R0) * (i / NSEG);
    const r1 = R0 + (R1 - R0) * ((i + 1) / NSEG);
    // Rim points: rotate the blade direction (-Y) toward +Z through the fan.
    const p0y = -Math.cos(a0) * r0, p0z = Math.sin(a0) * r0;
    const p1y = -Math.cos(a1) * r1, p1z = Math.sin(a1) * r1;
    verts[v++] = ox; verts[v++] = oy; verts[v++] = oz;
    verts[v++] = 0; verts[v++] = p0y; verts[v++] = p0z;
    verts[v++] = 0; verts[v++] = p1y; verts[v++] = p1z;
    // Additive blending: darker = more transparent. Bright at the origin and
    // the leading edge, dying off along the trail.
    const b0 = 0.95 * (1 - (i / NSEG) * 0.85);
    const b1 = 0.95 * (1 - ((i + 1) / NSEG) * 0.85);
    cols[c++] = 0.9; cols[c++] = 0.9; cols[c++] = 0.9;
    cols[c++] = b0; cols[c++] = b0; cols[c++] = b0;
    cols[c++] = b1; cols[c++] = b1; cols[c++] = b1;
  }

  const geom = new THREE.BufferGeometry();
  geom.setAttribute('position', new THREE.BufferAttribute(verts, 3));
  geom.setAttribute('color', new THREE.BufferAttribute(cols, 3));
  geom.computeBoundingSphere();

  const mat = new THREE.MeshBasicMaterial({
    vertexColors: true,
    transparent: true,
    opacity: 0,
    blending: THREE.AdditiveBlending,
    depthWrite: false,
    side: THREE.DoubleSide,
  });
  mat.color.set(rig.trailColor !== undefined && rig.trailColor !== null ? rig.trailColor : 0xffffff);

  const mesh = new THREE.Mesh(geom, mat);
  mesh.visible = false;
  mesh.frustumCulled = false; // one tiny mesh; never let culling pop a swing
  mesh.castShadow = false;
  mesh.receiveShadow = false;
  mesh.renderOrder = 6;
  rig.hand.add(mesh);

  A.trailMesh = mesh;
  A.trailMat = mat;
}

/* ------------------------------------------------------------------ pose */

/** poseRig computes the entire skeleton for this frame. Layers, in order:
 *  locomotion/idle base → guard mix → one-shot act mix → flinch additive →
 *  downed sprawl mix → low-pass → write to the rig. */
export function poseRig(rig, state, time, dt, THREE) {
  const A = rig._anim;
  if (!A) return;
  if (A.trailPending && THREE) buildTrail(rig, A, THREE);

  // Sanitize the clock: a NaN or a tab-switch mega-dt must never poison the
  // springs or the phase accumulators.
  if (!(dt > 0)) dt = 1 / 60;
  if (dt > 0.05) dt = 0.05;
  if (!(time > -1e12 && time < 1e12)) time = 0;

  const moving = !!state.moving, running = !!state.running;
  const guarding = !!state.guarding, downed = !!state.downed;

  /* -- smoothed weights ------------------------------------------------- */
  rig.moveW += ((moving ? 1 : 0) - rig.moveW) * Math.min(1, dt * 10);
  rig.guardW += ((guarding ? 1 : 0) - rig.guardW) * Math.min(1, dt * 14);
  rig.downedW += ((downed ? 1 : 0) - rig.downedW) * Math.min(1, dt * 6);
  A.runW += (((moving && running) ? 1 : 0) - A.runW) * Math.min(1, dt * 8);
  if (moving) rig.walkPhase += dt * (running ? 13 : 9); // tile-glide cadence

  const moveW = rig.moveW, guardW = rig.guardW, dw = rig.downedW, runW = A.runW;
  const idleW = (1 - moveW) * (1 - guardW) * (1 - dw);

  /* -- base layer: locomotion + idle ------------------------------------ */
  const sw = Math.sin(rig.walkPhase);
  const stride = (0.55 + runW * 0.32) * moveW;

  // Legs scissor on the two-beat; torso bobs at double frequency (one dip per
  // footfall) so the knee-less legs still read as bearing weight.
  let legLX = sw * stride;
  let legRX = -sw * stride;

  // Arms counter-swing WITH the elbow: the forearm bends hardest when its arm
  // swings forward (a real arm pump), and carries a standing bend at a run.
  const armAmp = (0.48 + runW * 0.3) * moveW;
  let armLX = -sw * armAmp;
  let armRX = sw * armAmp;
  let armLZ = 0.06, armRZ = -0.06;
  let foreLX = -0.22 - Math.max(0, -armLX) * (0.9 + runW * 0.7) - runW * 0.55 * moveW;
  let foreRX = -0.22 - Math.max(0, -armRX) * (0.9 + runW * 0.7) - runW * 0.55 * moveW;

  let torsoX = (0.05 + runW * 0.17) * moveW;      // lean into the run
  let torsoY = sw * 0.09 * moveW;                 // shoulder counter-rotation
  let torsoPY = -(0.5 - 0.5 * Math.cos(rig.walkPhase * 2))
    * (0.018 + runW * 0.022) * moveW;             // footfall dip
  let rootRX = 0, rootRZ = 0, rootPY = 0;
  let headX = 0, headY = 0;
  let handRX = 0, handRZ = 0;

  // Cloak spring target: drapes slightly back at rest, streams out with
  // speed, flutters on the footfall beat.
  let cloakT = 0.08 + moveW * (0.42 + runW * 0.5)
    + Math.sin(rig.walkPhase * 2) * 0.05 * moveW;

  // Idle: breath in the chest, a slow weight shift through the hips, the
  // sword hand resting near the hip, the head drifting as if watching the
  // field. All gated by idleW so it dissolves under any other verb.
  const br = Math.sin(time * 1.6);
  const shift = Math.sin(time * 0.7);
  torsoPY += br * 0.008 * idleW;
  torsoX += (0.02 + br * 0.018) * idleW;
  torsoY += shift * 0.045 * idleW;
  armLX += Math.sin(time * 1.6 + 0.5) * 0.04 * idleW;
  armRX += (-0.16 + br * 0.03) * idleW;   // hand riding at the hip
  foreRX += -0.42 * idleW;                // blade angled back at rest
  armRZ += -0.10 * idleW;
  foreLX += -0.12 * idleW;
  legLX += shift * 0.03 * idleW;
  legRX += -shift * 0.03 * idleW;
  headY += Math.sin(time * 0.5 + 1.7) * 0.06 * idleW;
  headX += br * 0.015 * idleW;

  /* -- guard stance (blend weight, alive under it) ---------------------- */
  if (guardW > 0.001) {
    const gSway = Math.sin(time * 2.3) * 0.045 + Math.sin(time * 3.7) * 0.02;
    const gLeg = guardW * (1 - moveW); // feet keep walking if guard-walking
    armRX = mix(armRX, -1.55 + gSway * 0.7, guardW);
    armRZ = mix(armRZ, -0.5, guardW);
    foreRX = mix(foreRX, -1.15, guardW);   // blade up ACROSS the body
    handRX = mix(handRX, -0.3, guardW);
    armLX = mix(armLX, -0.88, guardW);     // off-arm raised
    armLZ = mix(armLZ, 0.32, guardW);
    foreLX = mix(foreLX, -1.35, guardW);
    torsoX = mix(torsoX, 0.14, guardW);
    torsoY = mix(torsoY, 0.16 + gSway, guardW); // bladed stance, breathing
    torsoPY = mix(torsoPY, -0.035, guardW);     // slight crouch
    legLX = mix(legLX, -0.26, gLeg);
    legRX = mix(legRX, 0.3, gLeg);
  }

  // Counter-look: the head fights the torso to keep the gaze on target.
  headY -= torsoY * 0.5;
  headX -= torsoX * 0.4;

  /* -- one-shot acts ----------------------------------------------------- */
  let trailA = 0;

  // Edge-detect a new act for impulse effects (the dodge whips the cloak).
  if (rig.act !== A.lastAct) {
    if (rig.act && rig.act.kind === 'dodge') A.cloakV += 9;
    A.lastAct = rig.act;
  }

  const act = rig.act;
  if (act) {
    const dur = DUR[act.kind] || 0.3;
    let p = (time - act.start) / dur;
    if (p >= 1) {
      rig.act = null;
      A.lastAct = null;
    } else {
      if (!(p > 0)) p = 0;
      const m = act.mirror ? -1 : 1;

      switch (act.kind) {
        case 'fast': {
          // 0-30%: anticipation — the blade SNAPS up and back, torso winds.
          // 30-52%: the cut — easeIn arrives at max velocity and stops dead:
          //         that hard stop, held to 62%, is the two-frame contact.
          // 62-100%: settle back into whatever the base layer is doing.
          const w = p < 0.05 ? p / 0.05 : p < 0.66 ? 1 : 1 - sstep((p - 0.66) / 0.34);
          let aRX, aRZ, aFX, aTX, aTY, aHY, aHZ;
          if (p < 0.30) {
            const e = easeOut(p / 0.30);
            aRX = -2.35 * e; aRZ = m * 0.55 * e; aFX = -1.5 * e;
            aTY = -m * 0.42 * e; aTX = -0.06 * e; aHY = m * 0.3 * e;
            aHZ = -m * 0.3 * e;
          } else {
            const s = easeIn((p - 0.30) / 0.22); // hits 1 at p=0.52, then holds
            aRX = mix(-2.35, 0.85, s);            // diagonal cut, through
            aRZ = mix(m * 0.55, -m * 0.6, s);
            aFX = mix(-1.5, -0.08, s);            // elbow whips to extension
            aTY = mix(-m * 0.42, m * 0.5, s);     // torso rotates into it
            aTX = mix(-0.06, 0.16, s);
            aHY = mix(m * 0.3, -m * 0.15, s);
            aHZ = mix(-m * 0.3, m * 0.25, s);
          }
          armRX = mix(armRX, aRX, w);
          armRZ = mix(armRZ, aRZ, w);
          foreRX = mix(foreRX, aFX, w);
          torsoX = mix(torsoX, aTX, w);
          torsoY = mix(torsoY, aTY, w);
          headY = mix(headY, aHY, w);
          handRZ = mix(handRZ, aHZ, w);
          armLX = mix(armLX, 0.25 * Math.sin(p * Math.PI), w * 0.6); // counter
          legLX = mix(legLX, -0.16, w * 0.4 * (1 - moveW));          // brace
          legRX = mix(legRX, 0.14, w * 0.4 * (1 - moveW));
          if (p > 0.30) trailA = Math.sin(clamp01((p - 0.30) / 0.42) * Math.PI) * 0.9;
          break;
        }

        case 'strong': {
          // 0-55%: the telegraph — arm coiled high behind the head, torso
          //        wound tight, front leg braced, eyes still on the target.
          // 55-80%: the drop — easeIn commits everything, blade carries PAST
          //         the target, weight drives into the ground.
          // 80-100%: heavy recovery — the pose un-crushes slowly.
          const w = p < 0.05 ? p / 0.05 : p < 0.78 ? 1 : 1 - sstep((p - 0.78) / 0.22);
          const m2 = m;
          let aRX, aRZ, aFX, aLX, aTX, aTY, aHX, aHY, aPY, aLegL, aLegR;
          if (p < 0.55) {
            const e = easeOut(p / 0.55);
            aRX = -3.05 * e; aRZ = m2 * 0.3 * e; aFX = -1.85 * e;
            aLX = -0.55 * e;
            aTY = -m2 * 0.55 * e; aTX = -0.17 * e;
            aHX = -0.12 * e; aHY = m2 * 0.45 * e;   // gaze stays on target
            aPY = -0.02 * e;
            aLegL = -0.42 * e; aLegR = 0.34 * e;    // front leg braced
          } else if (p < 0.80) {
            const s = easeIn((p - 0.55) / 0.25);
            aRX = mix(-3.05, 1.25, s);              // overhead, THROUGH, past
            aRZ = mix(m2 * 0.3, -m2 * 0.35, s);
            aFX = mix(-1.85, -0.12, s);
            aLX = mix(-0.55, 0.35, s);
            aTY = mix(-m2 * 0.55, m2 * 0.5, s);
            aTX = mix(-0.17, 0.5, s);               // committed forward
            aHX = mix(-0.12, 0.4, s);
            aHY = mix(m2 * 0.45, -m2 * 0.1, s);
            aPY = mix(-0.02, -0.07, s);             // weight into the ground
            aLegL = mix(-0.42, -0.2, s);
            aLegR = mix(0.34, 0.55, s);
          } else {
            const r = sstep((p - 0.80) / 0.20);
            aRX = mix(1.25, 0.9, r);
            aRZ = -m2 * 0.35;
            aFX = mix(-0.12, -0.35, r);
            aLX = mix(0.35, 0.1, r);
            aTY = m2 * 0.5 * (1 - r * 0.4);
            aTX = mix(0.5, 0.3, r);
            aHX = mix(0.4, 0.15, r);
            aHY = -m2 * 0.1;
            aPY = mix(-0.07, -0.03, r);
            aLegL = -0.2; aLegR = mix(0.55, 0.4, r);
          }
          armRX = mix(armRX, aRX, w);
          armRZ = mix(armRZ, aRZ, w);
          foreRX = mix(foreRX, aFX, w);
          armLX = mix(armLX, aLX, w);
          torsoX = mix(torsoX, aTX, w);
          torsoY = mix(torsoY, aTY, w);
          headX = mix(headX, aHX, w);
          headY = mix(headY, aHY, w);
          rootPY = mix(rootPY, aPY, w);
          legLX = mix(legLX, aLegL, w * (1 - moveW * 0.6));
          legRX = mix(legRX, aLegR, w * (1 - moveW * 0.6));
          if (p > 0.55) trailA = Math.sin(clamp01((p - 0.55) / 0.33) * Math.PI);
          break;
        }

        case 'dodge': {
          // Tuck-and-roll: root pitches forward through a partial roll with a
          // y dip, everything gathered tight, cloak whipping (impulse above).
          const arc = Math.sin(p * Math.PI);
          const w = p < 0.05 ? p / 0.05 : p < 0.8 ? 1 : 1 - sstep((p - 0.8) / 0.2);
          rootRX = mix(rootRX, 1.85 * arc, w);
          rootPY = mix(rootPY, -0.17 * arc, w);
          torsoX = mix(torsoX, 0.5 * arc, w);
          torsoPY = mix(torsoPY, -0.02 * arc, w);
          headX = mix(headX, 0.55 * arc, w);        // chin tucked
          legLX = mix(legLX, -1.15 * arc, w);       // knees gathered
          legRX = mix(legRX, -1.0 * arc, w);
          armLX = mix(armLX, -0.75 * arc, w);       // arms hugged in
          armRX = mix(armRX, -0.7 * arc, w);
          armLZ = mix(armLZ, 0.3 * arc, w);
          armRZ = mix(armRZ, -0.3 * arc, w);
          foreLX = mix(foreLX, -1.7 * arc, w);
          foreRX = mix(foreRX, -1.6 * arc, w);
          break;
        }

        case 'parry': {
          // A crisp upward flick FROM THE FOREARM, shoulders squaring to meet
          // the blow. Fast attack, most of the return left to the smoother.
          const e = p < 0.30 ? easeOut(p / 0.30)
            : 1 - easeIn((p - 0.30) / 0.70) * 0.8;
          const w = p < 0.04 ? p / 0.04 : p < 0.75 ? 1 : 1 - sstep((p - 0.75) / 0.25);
          armRX = mix(armRX, -1.45 - 0.5 * e, w);
          foreRX = mix(foreRX, -0.7 - 1.35 * e, w); // the flick lives here
          armRZ = mix(armRZ, -0.4 - 0.35 * e, w);
          handRX = mix(handRX, 0.3 * e, w);
          torsoY = mix(torsoY, 0, w * 0.85);        // shoulders square
          torsoX = mix(torsoX, 0.05, w * 0.6);
          armLX = mix(armLX, -0.5, w * 0.5);
          headX = mix(headX, -0.08 * e, w);
          break;
        }
      }
    }
  }

  /* -- flinch: sharp backward hit reaction, quadratic decay -------------- */
  const fl = rig.flinchUntil - time;
  if (fl > 0) {
    const f = Math.min(1, fl / 0.28);
    const fe = f * f;               // sharp at the hit, soft at the tail
    torsoX -= 0.42 * fe;
    headX -= 0.38 * fe;
    armLX -= 0.2 * fe;              // arms thrown slightly up-back
    armRX -= 0.15 * fe;
  }

  /* -- downed: ease into a sideways sprawl, everything slack ------------- */
  if (dw > 0.001) {
    rootRZ = mix(rootRZ, -1.42, dw);   // over onto the side
    rootRX = mix(rootRX, 0.12, dw);
    rootPY = mix(rootPY, 0.09, dw);    // hips off the dirt
    torsoX = mix(torsoX, 0.1, dw);
    torsoY = mix(torsoY, 0.15, dw);
    torsoPY = mix(torsoPY, 0, dw);
    headX = mix(headX, 0.1, dw);
    headY = mix(headY, 0.45, dw);      // head lolls
    armLX = mix(armLX, 0.35, dw);      // limbs slack, asymmetric
    armRX = mix(armRX, 0.25, dw);
    armLZ = mix(armLZ, 0.15, dw);
    armRZ = mix(armRZ, -0.3, dw);
    foreLX = mix(foreLX, -0.2, dw);
    foreRX = mix(foreRX, -0.15, dw);
    legLX = mix(legLX, 0.25, dw);
    legRX = mix(legRX, -0.15, dw);
    handRX = mix(handRX, 0, dw);
    handRZ = mix(handRZ, 0, dw);
    cloakT = mix(cloakT, 0.05, dw);
  }

  /* -- one fast low-pass over every channel ------------------------------ *
   * Rate 30: crisp enough to preserve the strike snap (settles in ~2-3
   * frames at 60fps), soft enough that a replaced act or a dropped guard
   * never pops. This is the interruption insurance for the whole system.   */
  const k = 1 - Math.exp(-dt * 30);
  const J = A.j;
  J.legLRX += (legLX - J.legLRX) * k;
  J.legRRX += (legRX - J.legRRX) * k;
  J.armLRX += (armLX - J.armLRX) * k;
  J.armLRZ += (armLZ - J.armLRZ) * k;
  J.armRRX += (armRX - J.armRRX) * k;
  J.armRRZ += (armRZ - J.armRRZ) * k;
  J.foreLRX += (foreLX - J.foreLRX) * k;
  J.foreRRX += (foreRX - J.foreRRX) * k;
  J.handRX += (handRX - J.handRX) * k;
  J.handRZ += (handRZ - J.handRZ) * k;
  J.torsoRX += (torsoX - J.torsoRX) * k;
  J.torsoRY += (torsoY - J.torsoRY) * k;
  J.torsoPY += (torsoPY - J.torsoPY) * k;
  J.headRX += (headX - J.headRX) * k;
  J.headRY += (headY - J.headRY) * k;
  J.rootRX += (rootRX - J.rootRX) * k;
  J.rootRZ += (rootRZ - J.rootRZ) * k;
  J.rootPY += (rootPY - J.rootPY) * k;

  /* -- write the skeleton ------------------------------------------------ */
  rig.legL.rotation.x = J.legLRX;
  rig.legR.rotation.x = J.legRRX;
  rig.armL.rotation.x = J.armLRX;
  rig.armL.rotation.z = J.armLRZ;
  rig.armR.rotation.x = J.armRRX;
  rig.armR.rotation.z = J.armRRZ;
  if (rig.foreL) rig.foreL.rotation.x = J.foreLRX;
  if (rig.foreR) rig.foreR.rotation.x = J.foreRRX;
  rig.hand.rotation.x = J.handRX;
  rig.hand.rotation.z = J.handRZ;
  rig.torso.rotation.x = J.torsoRX;
  rig.torso.rotation.y = J.torsoRY;
  rig.torso.position.y = A.torsoBaseY + J.torsoPY;
  rig.head.rotation.x = J.headRX;
  rig.head.rotation.y = J.headRY;
  rig.root.rotation.x = J.rootRX;
  rig.root.rotation.z = J.rootRZ;
  rig.root.position.y = A.rootBaseY + J.rootPY;

  /* -- cloak: spring-damped, carrying real momentum ---------------------- */
  A.cloakV += ((cloakT - A.cloakX) * 70 - A.cloakV * 10) * dt;
  A.cloakX += A.cloakV * dt;
  if (!(A.cloakX > -10 && A.cloakX < 10)) { A.cloakX = 0.08; A.cloakV = 0; } // belt+braces
  if (rig.cloak) {
    rig.cloak.rotation.x = A.cloakX;
    rig.cloak.rotation.z = Math.sin(time * 1.1) * 0.03
      + Math.sin(rig.walkPhase) * 0.05 * moveW
      + A.cloakV * 0.02; // a hint of sideways life when the spring is excited
  }

  /* -- swing trail ------------------------------------------------------- */
  if (A.trailMat) {
    A.trailMat.opacity = trailA;
    A.trailMesh.visible = trailA > 0.02;
  }
}
