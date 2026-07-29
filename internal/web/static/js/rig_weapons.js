/* Durst World held-weapon meshes (art contract v2, "the armored duelist").
 *
 * One builder per silhouette class from internal/game/weapon_sprites.go. Local
 * space per the mount contract: the GRIP is at the origin and the business end
 * extends -Y, hanging along a lowered arm and arcing with a swing. Everything
 * is three.js r160 CORE — Shape/Extrude/Lathe/primitives, no addons, no
 * textures. Detail comes from silhouette and material breaks, so each weapon
 * still reads at 40 pixels: the sword is "cross + taper", the spear is "long
 * line + leaf", the bow is "double recurve + string", the dagger is "jagged
 * wedge", the sling is "cord + lump".
 *
 * flatShading is on everywhere (the polygonal-storybook world language), so
 * lathes and extrude bevels facet instead of smoothing. Budget ≤600 tris per
 * weapon; every build below sits between ~120 and ~420. All colors are fixed —
 * weapons are never player-tinted. castShadow on every mesh.
 *
 * Legends (Durstbane, Skypiercer) glow: emissive rune/edge accents and a
 * slightly grander silhouette, so the one-per-world find shimmers from afar.
 */

/* Fixed weapon palette. Warm woods echo the tileset trunk (#6B4A2B); the
 * legend sky-blue is the world's Accent2 cyan so the myth reads on-brand. */
const C = {
  steel: 0xd9dee6,
  steelDark: 0x8f99a4, // fuller / midrib shadow line
  guardGold: 0xa5813f,
  wood: 0x8a6134,
  woodDark: 0x6b4a2b,
  leather: 0x5f4026,
  leatherDark: 0x46301c,
  bone: 0xe9e1cc,
  flint: 0x565d68,
  stone: 0x8d949c,
  string: 0xd9d4c6,
  fletch: 0xe6ebf0,
  gold: 0xffd75e,
  goldEmissive: 0xffc23d,
  sky: 0x7df0ff,
  skyEmissive: 0x59d7ff,
};

/** buildWeapon returns a THREE.Group for a silhouette class, or null for an
 *  unknown one. THREE is passed in (the vendored r160 module); no imports. */
export function buildWeapon(cls, THREE) {
  switch (cls) {
    case 'blade': return buildBlade(THREE, false);
    case 'blade_legend': return buildBlade(THREE, true);
    case 'blade_s': return buildShortBlade(THREE);
    case 'polearm': return buildSpear(THREE);
    case 'bow': return buildBow(THREE, false);
    case 'bow_legend': return buildBow(THREE, true);
    case 'sling': return buildSling(THREE);
    default: return null;
  }
}

/** weaponAccent is the hex color a weapon's swing trail should tint toward:
 *  steel-white for blades, warm wood for the hafted arms, legend gold and
 *  storm sky-blue for the two myths. */
export function weaponAccent(cls) {
  switch (cls) {
    case 'blade':
    case 'blade_s': return 0xe8edf3;
    case 'polearm':
    case 'bow': return 0xa9743e;
    case 'sling': return 0x6b4a2f;
    case 'blade_legend': return C.gold;
    case 'bow_legend': return C.sky;
    default: return 0xe8edf3;
  }
}

/* ---------------------------------------------------------------- helpers */

function mat(THREE, color, roughness, metalness, extra) {
  return new THREE.MeshStandardMaterial(Object.assign(
    { color, roughness, metalness, flatShading: true }, extra || {}));
}

/** adder binds a group so each part is one line: geometry, material, done.
 *  Every mesh casts a shadow — a held sword must darken the ground. */
function adder(THREE, group) {
  return (geom, material) => {
    const mesh = new THREE.Mesh(geom, material);
    mesh.castShadow = true;
    group.add(mesh);
    return mesh;
  };
}

/** ridgedGrip lathes a leather-wrapped handle: the radius alternates so the
 *  cord wraps read as geometry ridges, not texture. Spans yBottom..yTop. */
function ridgedGrip(THREE, rMin, rMax, yBottom, yTop, wraps) {
  const pts = [];
  const n = wraps * 2; // alternate thin/thick down the length
  for (let i = 0; i <= n; i++) {
    const t = i / n;
    const y = yBottom + (yTop - yBottom) * t;
    const end = i === 0 || i === n;
    pts.push(new THREE.Vector2(end ? rMin * 0.94 : (i % 2 ? rMax : rMin), y));
  }
  return new THREE.LatheGeometry(pts, 6);
}

/** taperedBlade extrudes the classic arming-sword silhouette: straight
 *  shoulders, a long gentle taper, a curved point. Bevel gives the faceted
 *  edge-grind; a raised contrast rib (added by the caller) is the fuller. */
function taperedBlade(THREE, len, halfW, depth, bevel) {
  const s = new THREE.Shape();
  s.moveTo(-halfW, 0);
  s.lineTo(-halfW * 0.82, -len * 0.76);
  s.quadraticCurveTo(-halfW * 0.5, -len * 0.94, 0, -len);
  s.quadraticCurveTo(halfW * 0.5, -len * 0.94, halfW * 0.82, -len * 0.76);
  s.lineTo(halfW, 0);
  s.closePath();
  const g = new THREE.ExtrudeGeometry(s, {
    depth, curveSegments: 4,
    bevelEnabled: true, bevelThickness: bevel, bevelSize: bevel, bevelSegments: 1,
  });
  g.translate(0, 0, -depth / 2);
  return g;
}

/* ----------------------------------------------------- blade / Durstbane */

/** The Cast Blade — an arming sword: weighted pommel, ridged leather grip,
 *  a proper crossguard with knobbed ends, and a tapered blade carrying a
 *  fuller line. Durstbane is the same bones grown grander: longer and wider,
 *  swept guard wings, a gem in the pommel, and the fuller burning gold —
 *  "the blade that ended the long audit". */
function buildBlade(THREE, legend) {
  const g = new THREE.Group();
  const add = adder(THREE, g);

  const bladeLen = legend ? 0.50 : 0.42;
  const halfW = legend ? 0.037 : 0.030;
  const guardW = legend ? 0.23 : 0.19;

  const steel = legend
    ? mat(THREE, 0xe8e2d2, 0.28, 0.7, { emissive: C.goldEmissive, emissiveIntensity: 0.18 })
    : mat(THREE, C.steel, 0.3, 0.65);
  const gold = legend
    ? mat(THREE, C.guardGold, 0.35, 0.65, { emissive: C.goldEmissive, emissiveIntensity: 0.3 })
    : mat(THREE, C.guardGold, 0.35, 0.65);
  const leather = mat(THREE, C.leather, 0.9, 0);
  const fullerMat = legend
    ? mat(THREE, C.gold, 0.35, 0.4, { emissive: C.goldEmissive, emissiveIntensity: 1.4 })
    : mat(THREE, C.steelDark, 0.4, 0.6);

  // Pommel — the counterweight; a faceted ball above the fist.
  const pommel = new THREE.SphereGeometry(legend ? 0.033 : 0.029, 6, 5);
  pommel.translate(0, 0.098, 0);
  add(pommel, steel);
  if (legend) { // a rune-gem crowning the pommel, the distant shimmer's seed
    const gem = new THREE.OctahedronGeometry(0.018, 0);
    gem.translate(0, 0.13, 0);
    add(gem, fullerMat);
  }

  // Grip — leather wrap with real ridges, the hand closing over the origin.
  add(ridgedGrip(THREE, 0.0165, 0.021, -0.055, 0.078, 4), leather);

  // Crossguard — the bar plus end knobs; the 40-pixel "cross" read.
  const bar = new THREE.BoxGeometry(guardW, 0.026, 0.042);
  bar.translate(0, -0.07, 0);
  add(bar, gold);
  for (const sx of [-1, 1]) {
    const knob = new THREE.SphereGeometry(0.017, 5, 4);
    knob.translate(sx * guardW / 2, -0.07, 0);
    add(knob, gold);
    if (legend) { // swept wings angling down the blade — the grander cross
      const wing = add(new THREE.BoxGeometry(0.055, 0.019, 0.034), gold);
      wing.position.set(sx * (guardW / 2 - 0.008), -0.085, 0);
      wing.rotation.z = -sx * 0.55;
    }
  }

  // Ricasso collar seating the blade into the guard.
  const collar = new THREE.BoxGeometry(0.05, 0.03, 0.032);
  collar.translate(0, -0.093, 0);
  add(collar, steel);

  // Blade — beveled taper; the fuller is a raised contrast rib down the
  // center (a groove would cost holes; the dark/glowing line reads the same).
  const blade = taperedBlade(THREE, bladeLen, halfW, 0.016, 0.005);
  blade.translate(0, -0.088, 0);
  add(blade, steel);
  const fuller = new THREE.BoxGeometry(0.007, bladeLen * 0.62, 0.03);
  fuller.translate(0, -0.088 - bladeLen * 0.36, 0);
  add(fuller, fullerMat);

  return g;
}

/* --------------------------------------------------------------- blade_s */

/** Flint Knife / Bone Dagger — one wicked short blade for both: a knapped
 *  flint edge with serration notches on the spine, hafted into a bone grip
 *  lashed with leather. Quick in the hand, mean in the silhouette. */
function buildShortBlade(THREE) {
  const g = new THREE.Group();
  const add = adder(THREE, g);

  const bone = mat(THREE, C.bone, 0.55, 0.05);
  const flint = mat(THREE, C.flint, 0.55, 0.15);
  const leather = mat(THREE, C.leatherDark, 0.9, 0);

  // Bone grip: a knuckled lathe with a joint-knob pommel.
  const knob = new THREE.SphereGeometry(0.019, 5, 4);
  knob.translate(0, 0.068, 0);
  add(knob, bone);
  add(ridgedGrip(THREE, 0.013, 0.016, -0.035, 0.06, 3), bone);

  // Leather lashing where blade meets bone — two cord rings.
  for (const y of [0.014, -0.006]) {
    const ring = new THREE.CylinderGeometry(0.0175, 0.0175, 0.011, 6, 1, true);
    ring.translate(0, y, 0);
    add(ring, leather);
  }

  // A bone disc for a guard — barely there, as a skinning knife should be.
  const disc = new THREE.CylinderGeometry(0.026, 0.028, 0.014, 6);
  disc.translate(0, -0.04, 0);
  add(disc, bone);

  // The flint blade: smooth cutting edge, notched spine, knapped facets
  // from the extrude bevel under flat shading.
  const s = new THREE.Shape();
  s.moveTo(-0.019, 0);
  s.lineTo(-0.0225, -0.045);
  s.lineTo(-0.015, -0.055);  // serration notch
  s.lineTo(-0.0205, -0.095);
  s.lineTo(-0.012, -0.104);  // serration notch
  s.lineTo(-0.015, -0.145);
  s.quadraticCurveTo(-0.007, -0.175, 0, -0.195); // the point
  s.quadraticCurveTo(0.013, -0.15, 0.019, -0.105);
  s.quadraticCurveTo(0.023, -0.05, 0.017, 0);
  s.closePath();
  const blade = new THREE.ExtrudeGeometry(s, {
    depth: 0.012, curveSegments: 4,
    bevelEnabled: true, bevelThickness: 0.003, bevelSize: 0.003, bevelSegments: 1,
  });
  blade.translate(0, -0.044, -0.006);
  add(blade, flint);

  return g;
}

/* --------------------------------------------------------------- polearm */

/** The Spear — a long tapered haft held mid-shaft, a leaf-shaped head with a
 *  raised midrib, and a leather binding collar (with cord rings) where steel
 *  meets wood. Head down the -Y business end per the mount contract. */
function buildSpear(THREE) {
  const g = new THREE.Group();
  const add = adder(THREE, g);

  const steel = mat(THREE, C.steel, 0.3, 0.65);
  const steelDark = mat(THREE, C.steelDark, 0.4, 0.6);
  const wood = mat(THREE, C.wood, 0.8, 0);
  const leather = mat(THREE, C.leather, 0.9, 0);
  const cord = mat(THREE, C.leatherDark, 0.9, 0);

  // Haft: slightly wider toward the head so the taper leads the eye down.
  const haft = new THREE.CylinderGeometry(0.015, 0.017, 0.95, 6);
  haft.translate(0, -0.025, 0); // grip mid-haft; butt rides high behind
  add(haft, wood);
  const butt = new THREE.CylinderGeometry(0.017, 0.013, 0.035, 6);
  butt.translate(0, 0.465, 0);
  add(butt, steelDark);

  // Binding collar: a leather sleeve pinched by two cord rings.
  const sleeve = new THREE.CylinderGeometry(0.024, 0.026, 0.075, 6);
  sleeve.translate(0, -0.487, 0);
  add(sleeve, leather);
  for (const y of [-0.459, -0.514]) {
    const ring = new THREE.CylinderGeometry(0.027, 0.027, 0.008, 6, 1, true);
    ring.translate(0, y, 0);
    add(ring, cord);
  }

  // Leaf head: widest a third down, both ends drawing to a point; the
  // extrude bevel grinds the edge, the dark midrib is the forging line.
  const s = new THREE.Shape();
  s.moveTo(0, 0.03); // tang, buried in the collar
  s.quadraticCurveTo(-0.042, -0.02, -0.035, -0.085);
  s.quadraticCurveTo(-0.02, -0.15, 0, -0.2);
  s.quadraticCurveTo(0.02, -0.15, 0.035, -0.085);
  s.quadraticCurveTo(0.042, -0.02, 0, 0.03);
  const head = new THREE.ExtrudeGeometry(s, {
    depth: 0.013, curveSegments: 5,
    bevelEnabled: true, bevelThickness: 0.004, bevelSize: 0.004, bevelSegments: 1,
  });
  head.translate(0, -0.525, -0.0065);
  add(head, steel);
  const rib = new THREE.BoxGeometry(0.006, 0.15, 0.026);
  rib.translate(0, -0.61, 0);
  add(rib, steelDark);

  return g;
}

/* --------------------------------------------------------- bow / Skypiercer */

/** bowLimb extrudes one recurve limb as a closed outline in the XY plane:
 *  the limb sweeps away from the string (belly toward -X, the shooting side)
 *  then hooks back at the tip — the double-S that says "recurve" at any
 *  distance. sgn mirrors it for the lower limb; k scales the legend up. */
function bowLimb(THREE, sgn, k) {
  const s = new THREE.Shape();
  const m = (x, y) => s.moveTo(x * k, y * sgn * k);
  const l = (x, y) => s.lineTo(x * k, y * sgn * k);
  const q = (cx, cy, x, y) => s.quadraticCurveTo(cx * k, cy * sgn * k, x * k, y * sgn * k);
  m(-0.020, 0.05);              // back of the limb, leaving the riser
  q(-0.072, 0.16, -0.050, 0.265);
  q(-0.030, 0.345, 0.070, 0.385); // the recurve hook, out to the tip
  l(0.082, 0.358);              // nock face (the string seats here)
  q(0.005, 0.315, -0.024, 0.240); // belly side, back down
  q(-0.046, 0.150, 0.006, 0.055);
  l(0.010, 0.05);
  s.closePath();
  const g = new THREE.ExtrudeGeometry(s, { depth: 0.02, curveSegments: 6, bevelEnabled: false });
  g.translate(0, 0, -0.01);
  return g;
}

/** Hunter's Bow — recurve limbs, a leather-wrapped grip riser, a taut string
 *  inboard (+X, toward the archer) and a nocked arrow pointing -X, ready.
 *  Skypiercer is the legend: grander limbs, storm-sinew string and arrowhead
 *  burning sky-blue, rune plates on the limbs and glowing nock caps. */
function buildBow(THREE, legend) {
  const g = new THREE.Group();
  const add = adder(THREE, g);
  const k = legend ? 1.13 : 1;

  const wood = mat(THREE, legend ? C.woodDark : C.wood, 0.8, 0);
  const leather = mat(THREE, C.leather, 0.9, 0);
  const band = mat(THREE, C.leatherDark, 0.9, 0);
  const string = legend
    ? mat(THREE, 0xbfefff, 0.5, 0, { emissive: C.sky, emissiveIntensity: 1.2 })
    : mat(THREE, C.string, 0.85, 0);
  const head = legend
    ? mat(THREE, 0x9df3ff, 0.35, 0.4, { emissive: C.skyEmissive, emissiveIntensity: 1.4 })
    : mat(THREE, C.steel, 0.3, 0.65);
  const glow = legend
    ? mat(THREE, 0x9df3ff, 0.35, 0.4, { emissive: C.skyEmissive, emissiveIntensity: 1.4 })
    : null;

  // Limbs, mirrored about the grip.
  add(bowLimb(THREE, 1, k), wood);
  add(bowLimb(THREE, -1, k), wood);

  // Grip riser: a leather block bridging the limb roots, banded top and
  // bottom so the wrap reads as a material break.
  const riser = new THREE.BoxGeometry(0.034 * k, 0.115 * k, 0.036 * k);
  riser.translate(-0.005 * k, 0, 0);
  add(riser, leather);
  for (const sy of [-1, 1]) {
    const wrap = new THREE.BoxGeometry(0.038 * k, 0.014 * k, 0.04 * k);
    wrap.translate(-0.005 * k, sy * 0.042 * k, 0);
    add(wrap, band);
  }

  // The string, nock to nock. Taut — the bow is always ready in this world.
  const str = new THREE.BoxGeometry(0.005, 0.744 * k, 0.005);
  str.translate(0.076 * k, 0, 0);
  add(str, string);

  // A nocked arrow along -X: shaft, faceted head, crossed fletching.
  const nockX = 0.076 * k;
  const shaft = new THREE.CylinderGeometry(0.0045, 0.0045, 0.3, 5);
  shaft.rotateZ(Math.PI / 2);
  shaft.translate(nockX - 0.15, 0, 0);
  add(shaft, wood);
  const tip = new THREE.ConeGeometry(0.011, 0.038, 5);
  tip.rotateZ(Math.PI / 2); // +Y cone spun to point -X, downrange
  tip.translate(nockX - 0.318, 0, 0);
  add(tip, head);
  const fletchA = new THREE.BoxGeometry(0.032, 0.016, 0.0025);
  fletchA.translate(nockX - 0.045, 0, 0);
  add(fletchA, mat(THREE, C.fletch, 0.8, 0));
  const fletchB = new THREE.BoxGeometry(0.032, 0.0025, 0.016);
  fletchB.translate(nockX - 0.045, 0, 0);
  add(fletchB, mat(THREE, C.fletch, 0.8, 0));

  if (legend) {
    // Storm furniture: glowing nock caps, rune plates set through each limb,
    // and a gem on the riser's belly — the shimmer that draws players in.
    for (const sy of [-1, 1]) {
      const cap = new THREE.BoxGeometry(0.02 * k, 0.026 * k, 0.026 * k);
      cap.translate(0.072 * k, sy * 0.372 * k, 0);
      add(cap, glow);
      const rune = new THREE.BoxGeometry(0.01 * k, 0.032 * k, 0.026 * k);
      rune.translate(-0.041 * k, sy * 0.155 * k, 0);
      add(rune, glow);
    }
    const gem = new THREE.OctahedronGeometry(0.016 * k, 0);
    gem.translate(-0.028 * k, 0, 0);
    add(gem, glow);
  }

  return g;
}

/* ----------------------------------------------------------------- sling */

/** The Sling — a finger loop at the grip, twin leather straps falling to a
 *  flattened pouch cradling a gathered stone. Humble on purpose: the found
 *  starter arm should look like something pulled from a hedge-witch's pack. */
function buildSling(THREE) {
  const g = new THREE.Group();
  const add = adder(THREE, g);

  const leather = mat(THREE, C.leather, 0.9, 0);
  const cord = mat(THREE, C.leatherDark, 0.9, 0);
  const stone = mat(THREE, C.stone, 0.8, 0.05);

  // Finger loop, held at the origin.
  const loop = new THREE.TorusGeometry(0.014, 0.0045, 4, 8);
  loop.translate(0, 0.004, 0);
  add(loop, cord);

  // Twin straps, splayed a hair so the double cord reads in silhouette.
  for (const sx of [-1, 1]) {
    const strapGeom = new THREE.BoxGeometry(0.02, 0.31, 0.007);
    strapGeom.translate(0, -0.155, 0);
    const strap = add(strapGeom, leather);
    strap.rotation.z = sx * 0.09;
  }

  // The pouch: a squashed faceted sphere, wider than tall — a cradle.
  const pouch = new THREE.SphereGeometry(0.048, 7, 5);
  pouch.scale(1.15, 0.62, 0.85);
  pouch.translate(0, -0.315, 0);
  add(pouch, leather);

  // A gathered stone sitting proud of the pouch lip.
  const rock = new THREE.IcosahedronGeometry(0.024, 0);
  rock.translate(0, -0.292, 0);
  add(rock, stone);

  return g;
}
