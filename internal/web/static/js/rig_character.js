/* character.js — the ARMORED DUELIST: constructor body for the hero Rig.
 *
 * Design notes (read before editing):
 *
 * SILHOUETTE. Five stylized heads inside the 1.05-unit cap: broad layered
 * pauldrons at the widest point (x ±0.24..0.34), a cuirass that tapers hard
 * from chest to waist (a 4-segment cylinder rotated 45° — the cheapest
 * tapered box there is), and boots that widen again at the ground so the
 * figure stands like a keel, not a pin. The cloak breaks the back outline
 * with a flared swallow hem so facing reads even from behind.
 *
 * COLOR IS IDENTITY. The player's hue owns the readable area: tabard panels
 * front and back over the steel, sleeves, trousers, the darker cloak, and a
 * bright skin face. Steel and leather are fixed world colors, exactly as
 * props.js treats wood and stone. Every tinted material lands in rig.lit for
 * the night self-illumination pass.
 *
 * MATERIAL BREAKS, NOT TEXTURE. steel 0.35/0.6, leather 0.75/0.0, cloth
 * 0.85/0.0, skin brighter tint at 0.55 rough. flatShading everywhere so the
 * skull, pauldron domes, and cuirass facet like the icosahedron trees.
 *
 * CONTRACT COMPLIANCE. Every skeleton group of the animator contract is
 * created and parented here; limb geometry hangs BELOW its pivot (translated
 * -len/2) so group rotation swings like a joint; the figure faces +Z; the
 * hat is authored at world heights and re-based with -HEAD_PIVOT_Y under the
 * head group; skull center stays at y=0.85, r=0.15. When the hat accessory
 * is worn the steel skullcap is omitted (the hat IS the headwear) so the two
 * never interpenetrate.
 *
 * BUDGET. 16 meshes bare-headed, 17 with the hat (cap 18); well under 1000
 * triangles against the 2500 cap — the only real curvature is one 10×8
 * skull sphere and two 7-segment pauldron domes.
 */

export function buildDuelist(rig, color, accessory, THREE) {
  /* Joint pivots — must match the animator's expectations exactly. */
  const HIP_Y = 0.42;
  const SHOULDER_Y = 0.72;
  const SHOULDER_X = 0.24;
  const HEAD_PIVOT_Y = 0.74;
  const UPPER_LEN = 0.18;   // shoulder → elbow
  const FORE_LEN = 0.17;    // elbow → wrist (total reach matches old 0.34 arm)

  const STEEL = 0xd8dde2;   // the same steel as the weapons in rig.js
  const LEATHER = 0x6b4a2f; // the sling-strap leather from the world vocabulary

  if (!rig.lit) rig.lit = [];

  /* ---------- materials ---------- */

  const steelMat = new THREE.MeshStandardMaterial({
    color: STEEL, roughness: 0.35, metalness: 0.6, flatShading: true,
  });
  const leatherMat = new THREE.MeshStandardMaterial({
    color: LEATHER, roughness: 0.75, metalness: 0, flatShading: true,
  });

  /* Player-tinted cloth/skin; every one of these joins rig.lit. */
  const tinted = (t, rough = 0.85, extra = {}) => {
    const m = new THREE.MeshStandardMaterial({
      color: color.clone().multiplyScalar(t),
      roughness: rough, metalness: 0, flatShading: true, ...extra,
    });
    rig.lit.push(m);
    return m;
  };
  const tabardMat = tinted(1.0);                 // the loudest color read
  const sleeveMat = tinted(0.85);
  const trouserMat = tinted(0.8);
  const cloakMat = tinted(0.62, 0.85, { side: THREE.DoubleSide });
  const skinMat = tinted(1.15, 0.55);            // face, brightest tint

  /* ---------- small helpers ---------- */

  /** One shadowed mesh, parented. Solid armor casts and receives. */
  const solid = (geom, mat, parent) => {
    const mesh = new THREE.Mesh(geom, mat);
    mesh.castShadow = true;
    mesh.receiveShadow = true;
    parent.add(mesh);
    return mesh;
  };

  /** merge concatenates geometries sharing an attribute layout (all the core
   *  primitives used here emit position/normal/uv + index) — same hand-rolled
   *  approach as props.js, since core three has no BufferGeometryUtils. */
  const merge = (geoms) => {
    if (geoms.length === 1) return geoms[0];
    const out = new THREE.BufferGeometry();
    for (const name of Object.keys(geoms[0].attributes)) {
      const itemSize = geoms[0].attributes[name].itemSize;
      let total = 0;
      for (const g of geoms) total += g.attributes[name].array.length;
      const arr = new Float32Array(total);
      let off = 0;
      for (const g of geoms) {
        arr.set(g.attributes[name].array, off);
        off += g.attributes[name].array.length;
      }
      out.setAttribute(name, new THREE.BufferAttribute(arr, itemSize));
    }
    const idx = [];
    let base = 0;
    for (const g of geoms) {
      for (const v of g.index.array) idx.push(v + base);
      base += g.attributes.position.count;
    }
    out.setIndex(idx);
    for (const g of geoms) g.dispose();
    return out;
  };

  /** A tapered box: a 4-segment cylinder rotated 45° so its flats face the
   *  axes. wTop/wBot are full widths; depth is width × dScale. */
  const taperBox = (wTop, wBot, h, dScale) => {
    const g = new THREE.CylinderGeometry(
      wTop * Math.SQRT1_2, wBot * Math.SQRT1_2, h, 4, 1);
    g.rotateY(Math.PI / 4);
    g.scale(1, 1, dScale);
    return g;
  };

  const box = (w, h, d, x, y, z) => {
    const g = new THREE.BoxGeometry(w, h, d);
    g.translate(x, y, z);
    return g;
  };

  /* ---------- torso: cuirass, tabard, belt ---------- */

  /* Pivot at the hips so heavy swings put the back into it. Local y=0 is the
     hip line (world 0.42); shoulders land at local 0.30. */
  rig.torso = new THREE.Group();
  rig.torso.position.y = HIP_Y;
  rig.root.add(rig.torso);

  /* Steel: one merged mesh — cuirass tapering chest→waist, a horizontal
     chest-line bevel plus a center keel (the classic breastplate ridge), and
     the belt buckle riding proud of the belt as the front-facing cue. */
  const cuirass = taperBox(0.40, 0.26, 0.27, 0.60);
  cuirass.translate(0, 0.215, 0);                       // world 0.50 → 0.77
  solid(merge([
    cuirass,
    box(0.26, 0.04, 0.06, 0, 0.26, 0.105),              // chest-line bevel
    box(0.045, 0.14, 0.05, 0, 0.185, 0.11),             // center keel
    box(0.075, 0.075, 0.035, 0, 0.105, 0.10),           // belt buckle
  ]), steelMat, rig.torso);

  /* The tabard: player color front AND back, hanging over the belt line so
     the identity hue survives both cameras. */
  solid(merge([
    box(0.19, 0.40, 0.025, 0, 0.13, 0.125),             // front panel
    box(0.19, 0.32, 0.02, 0, 0.16, -0.115),             // back panel
  ]), tabardMat, rig.torso);

  /* Leather belt cinching the tapered waist. */
  solid(box(0.28, 0.055, 0.19, 0, 0.105, 0), leatherMat, rig.torso);

  /* ---------- head: skin face with a real front ---------- */

  rig.head = new THREE.Group();
  rig.head.position.y = HEAD_PIVOT_Y - HIP_Y;
  rig.torso.add(rig.head);

  /* Skull center at world 0.85, r 0.15 — the hat contract. Brow shelf and
     nose wedge are merged into the skin so the face self-shadows into a
     readable front at 30 pixels; facing is gameplay. */
  const skull = new THREE.SphereGeometry(0.15, 10, 8);
  skull.translate(0, 0.11, 0);
  solid(merge([
    skull,
    box(0.17, 0.035, 0.05, 0, 0.16, 0.115),             // brow line
    box(0.05, 0.05, 0.06, 0, 0.095, 0.145),             // nose wedge
  ]), skinMat, rig.head);

  if (!accessory) {
    /* Visor-less steel sallet: a skullcap plus a side/back rim band whose
       arc leaves the whole face open (cylinder theta 0 faces +Z). Skipped
       when a hat is worn — the hat replaces the helm. */
    const cap = new THREE.SphereGeometry(
      0.168, 9, 5, 0, Math.PI * 2, 0, Math.PI * 0.42);
    cap.translate(0, 0.115, -0.01);
    const band = new THREE.CylinderGeometry(
      0.165, 0.172, 0.05, 9, 1, true, Math.PI * 0.42, Math.PI * 1.16);
    band.translate(0, 0.175, -0.005);
    solid(merge([cap, band]), steelMat, rig.head);
  } else {
    /* Hat geometry carries world heights (brim at 0.97); re-base onto the
       head pivot so it rides a nod or a knock-out. Same hat as ever. */
    const hat = new THREE.Group();
    hat.position.y = -HEAD_PIVOT_Y;
    const brim = new THREE.CylinderGeometry(0.23, 0.25, 0.04, 10);
    brim.translate(0, 0.97, 0);
    solid(brim, tinted(0.9), hat);
    const crown = new THREE.CylinderGeometry(0.13, 0.15, 0.14, 10);
    crown.translate(0, 1.05, 0);
    solid(crown, tinted(1.3), hat);
    rig.head.add(hat);
  }

  /* ---------- arms: pauldrons, sleeves, bracer-gauntlets ---------- */

  /* buildArm makes one shoulder group. sign is -1 left / +1 right, so the
     pauldron layers shift outboard and mirror cleanly. Pauldrons parent to
     the shoulder group: they ride every swing, which sells the weight. */
  const buildArm = (sign) => {
    const arm = new THREE.Group();
    arm.position.set(sign * SHOULDER_X, SHOULDER_Y - HIP_Y, 0);
    rig.torso.add(arm);

    /* Layered pauldron: faceted dome over a flared skirt plate. */
    const dome = new THREE.SphereGeometry(
      0.115, 7, 4, 0, Math.PI * 2, 0, Math.PI * 0.55);
    dome.scale(1.2, 0.9, 1.05);
    dome.translate(sign * 0.02, 0.03, 0);
    const skirt = new THREE.CylinderGeometry(0.10, 0.13, 0.06, 7);
    skirt.translate(sign * 0.02, -0.025, 0);
    solid(merge([dome, skirt]), steelMat, arm);

    /* Upper-arm sleeve, hanging below the pivot like every limb. */
    solid(box(0.10, 0.16, 0.11, 0, -0.10, 0), sleeveMat, arm);

    /* Elbow group at the contract position; leather bracer plus a merged
       gauntlet fist hang below it. */
    const fore = new THREE.Group();
    fore.position.y = -UPPER_LEN;
    arm.add(fore);
    solid(merge([
      box(0.105, 0.15, 0.115, 0, -0.075, 0),            // bracer
      box(0.07, 0.065, 0.075, 0, -0.185, 0.005),        // gauntlet fist
    ]), leatherMat, fore);
    return { arm, fore };
  };

  const left = buildArm(-1);
  const right = buildArm(1);
  rig.armL = left.arm;
  rig.foreL = left.fore;
  rig.armR = right.arm;
  rig.foreR = right.fore;

  /* Weapon mount at the right wrist — grip at the origin per contract. */
  rig.hand = new THREE.Group();
  rig.hand.position.y = -FORE_LEN;
  rig.foreR.add(rig.hand);

  /* ---------- legs: trousers into knee-guard boots ---------- */

  const buildLeg = (sign) => {
    const leg = new THREE.Group();
    leg.position.set(sign * 0.09, HIP_Y, 0);
    rig.root.add(leg);

    /* Player-cloth cuisse from hip to knee. */
    solid(box(0.13, 0.20, 0.14, 0, -0.10, 0), trouserMat, leg);

    /* Leather boot: shaft to the ground (feet at world y=0), forward toe
       cap, and a knee cop proud of the shaft — the knee-guard silhouette. */
    solid(merge([
      box(0.13, 0.21, 0.14, 0, -0.315, 0),              // shaft
      box(0.115, 0.075, 0.10, 0, -0.3825, 0.095),       // toe cap (+Z front)
      box(0.12, 0.07, 0.05, 0, -0.205, 0.06),           // knee cop
    ]), leatherMat, leg);
    return leg;
  };

  rig.legL = buildLeg(-1);
  rig.legR = buildLeg(1);

  /* ---------- cloak ---------- */

  /* A tapered, flared panel pivoting at the shoulders (the animator sways
     the group, so the drape tilt is BAKED into the geometry and the group's
     rest rotation stays zero). Flares at mid-fall, nips back to a swallow
     hem — silhouette interest at zero triangle cost. DoubleSide because a
     cloak is seen from both faces; cloth can afford the flipped back-face
     normals that foliage can't. */
  rig.cloak = new THREE.Group();
  rig.cloak.position.set(0, SHOULDER_Y - HIP_Y, -0.11);
  rig.torso.add(rig.cloak);

  const cShape = new THREE.Shape();
  cShape.moveTo(-0.13, 0);
  cShape.lineTo(0.13, 0);
  cShape.lineTo(0.20, -0.40);
  cShape.lineTo(0.12, -0.55);
  cShape.lineTo(-0.12, -0.55);
  cShape.lineTo(-0.20, -0.40);
  cShape.closePath();
  const cloakGeom = new THREE.ShapeGeometry(cShape);
  cloakGeom.rotateX(0.18);      // baked drape: hem kicks back, clear of legs
  const cloakMesh = new THREE.Mesh(cloakGeom, cloakMat);
  cloakMesh.castShadow = true;
  rig.cloak.add(cloakMesh);
}
