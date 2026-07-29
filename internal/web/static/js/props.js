/* Prop geometry.
 *
 * The server sends a prop as a shape key ("tree:conifer", "building:cathedral")
 * plus a color. This module turns each key into a small pile of geometry.
 *
 * A shape is a list of *parts*, and each part becomes one InstancedMesh — so a
 * forest of 300 firs is six draw calls, not six hundred. That constraint shapes
 * the code below: parts bake their offset into the geometry (there is no
 * per-part transform at draw time), and per-part coloring is expressed as a
 * tint of the prop's own color, computed on the CPU when the instance is
 * placed. A roof is "the wall color, darker" rather than a second material.
 *
 * Nothing here knows what a fir *is* — only that the server called it
 * tree:conifer and gave it a green. That's what keeps the biome palette,
 * day/night and the art styles working without a second source of truth.
 */

import * as THREE from 'three';

/* Fixed colors for the few materials that aren't the prop's own: wood is wood
   whatever grows on it. WOOD matches the canonical trunk #6B4A2B from the
   tileset (this file once carried a typo'd near-miss, 0x6b4b32 — same tree,
   two woods). */
const WOOD = 0x6b4a2b;
const DARKWOOD = 0x4a3423;
const STONE = 0x8a8f96;
const THATCH = 0x9c7c4a;
const GLASS = 0xffd9a0;
const ACCENT = 0x7df0ff; // palette.Accent2 — the "this is a find" cue

/** part wraps a geometry with how it should be colored and lit. */
function part(geom, opts = {}) {
  return {
    geom,
    tint: opts.tint ?? 1,        // multiplier on the prop's color
    fixed: opts.fixed ?? null,   // an absolute color, ignoring the prop's
    glow: opts.glow ?? 0,        // emissive strength
    sway: opts.sway ?? 0,        // how much wind moves this part
    bob: opts.bob ?? false,      // hover-and-bob (pickups)
    rough: opts.rough,           // PBR roughness; undefined takes the default
    metal: opts.metal,           // PBR metalness
    // Draw this part from both sides. Nothing needs it today — foliage solves
    // the same problem with mirrored quads instead, because DoubleSide flips
    // normals on back faces and that's exactly what foliage can't afford (see
    // blades). Kept for a part that genuinely wants it.
    double: opts.double ?? false,
  };
}

/** vhash gives a builder a stable stream of [0,1) values for one variant, so
 *  variant 2 of a conifer is the same conifer everywhere, forever. */
function vhash(v, salt) {
  let h = Math.imul(v + 1, 668265263) ^ Math.imul(salt | 0, 374761393);
  h = Math.imul(h ^ (h >>> 13), 1274126177);
  return ((h ^ (h >>> 16)) >>> 0) / 4294967296;
}

/* Geometry helpers. Each returns geometry already positioned relative to the
   tile's center at ground level, so an instance only needs a translation. */

function box(w, h, d, x = 0, y = 0, z = 0) {
  const g = new THREE.BoxGeometry(w, h, d);
  g.translate(x, y + h / 2, z);
  return g;
}

function cyl(rTop, rBot, h, seg, x = 0, y = 0, z = 0) {
  const g = new THREE.CylinderGeometry(rTop, rBot, h, seg);
  g.translate(x, y + h / 2, z);
  return g;
}

function cone(r, h, seg, x = 0, y = 0, z = 0) {
  const g = new THREE.ConeGeometry(r, h, seg);
  g.translate(x, y + h / 2, z);
  return g;
}

function sphere(r, x = 0, y = 0, z = 0, widthSeg = 8, heightSeg = 6) {
  const g = new THREE.SphereGeometry(r, widthSeg, heightSeg);
  g.translate(x, y + r, z);
  return g;
}

/** A cylinder lying along an axis. Rotation happens before the translate, so
 *  the piece ends up where it was asked for rather than swung about the tile. */
function cylAxis(axis, r, len, seg, x = 0, y = 0, z = 0) {
  const g = new THREE.CylinderGeometry(r, r, len, seg);
  if (axis === 'x') g.rotateZ(Math.PI / 2);
  if (axis === 'z') g.rotateX(Math.PI / 2);
  g.translate(x, y + r, z);
  return g;
}

/** A cone pointing along +Z, for beaks and nozzles. */
function coneZ(r, h, seg, x = 0, y = 0, z = 0) {
  const g = new THREE.ConeGeometry(r, h, seg);
  g.rotateX(Math.PI / 2);
  g.translate(x, y, z);
  return g;
}

function blob(r, x = 0, y = 0, z = 0, detail = 0) {
  // An icosahedron reads as irregular stone at this scale and is far cheaper
  // than a sphere with enough segments to look rough.
  const g = new THREE.IcosahedronGeometry(r, detail);
  g.translate(x, y + r * 0.8, z);
  return g;
}

/** A pitched roof: a 4-sided pyramid, squashed to the building's footprint. */
function roof(w, d, h, y) {
  const g = new THREE.ConeGeometry(0.72, h, 4);
  g.rotateY(Math.PI / 4);
  g.scale(w, 1, d);
  g.translate(0, y + h / 2, 0);
  return g;
}

/** Crossed vertical quads — the cheapest thing that reads as grass or leaves.
 *
 *  Two details here are load-bearing, and both were bugs first.
 *
 *  The normals are forced to point straight up. A vertical quad under an
 *  overhead sun catches almost no light and renders as a black cut-out;
 *  lighting blades as though they were the ground they stand on is the standard
 *  foliage trick, and it keeps grass the same brightness as its tile.
 *
 *  Each quad is then emitted twice, back to back, and the material stays
 *  single-sided. The obvious way to make a quad visible from behind is
 *  DoubleSide — but three flips the normal on back faces, which turns that
 *  carefully-chosen up-normal into a down-normal and puts the grass right back
 *  in the dark. Mirrored pairs mean whichever face you're looking at is a front
 *  face, with the normal we asked for. */
function blades(w, h, n, y = 0) {
  const geoms = [];
  for (let i = 0; i < n; i++) {
    for (const flip of [0, Math.PI]) {
      const g = new THREE.PlaneGeometry(w, h);
      g.rotateY((i / n) * Math.PI + flip);
      g.translate(0, y + h / 2, 0);
      geoms.push(g);
    }
  }
  // Without BufferGeometryUtils (core three only) we merge by hand: the quads
  // are all PlaneGeometry, so their attributes line up exactly.
  const merged = mergeSame(geoms);
  const normals = merged.attributes.normal;
  for (let i = 0; i < normals.count; i++) normals.setXYZ(i, 0, 1, 0);
  normals.needsUpdate = true;
  return merged;
}

/** mergeSame concatenates geometries that share an attribute layout. */
function mergeSame(geoms) {
  if (geoms.length === 1) return geoms[0];
  const out = new THREE.BufferGeometry();
  const names = Object.keys(geoms[0].attributes);
  for (const name of names) {
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
  if (geoms[0].index) {
    const idx = [];
    let base = 0;
    for (const g of geoms) {
      for (const v of g.index.array) idx.push(v + base);
      base += g.attributes.position.count;
    }
    out.setIndex(idx);
  }
  for (const g of geoms) g.dispose();
  return out;
}

/* ---------- the builders ---------- */

const builders = {
  tree(s, v = 0) {
    const h = s.h;
    switch (s.style) {
      case 'conifer': {
        // A tapering trunk with a root flare, then five stacked cones — each
        // turned and nudged off-axis by the variant, with a thin spike on top,
        // so the silhouette is a fir rather than a stack of party hats.
        const parts = [part(cyl(0.05, 0.11, h * 0.4, 6), { fixed: DARKWOOD })];
        const tiers = 5;
        for (let i = 0; i < tiers; i++) {
          const t = i / tiers;
          const r = 0.46 * (1 - t * 0.62) * (0.9 + vhash(v, i) * 0.2);
          const g = cone(r, h * 0.30, 7, (vhash(v, i + 8) - 0.5) * 0.08,
            h * (0.16 + t * 0.15), (vhash(v, i + 16) - 0.5) * 0.08);
          g.rotateY(vhash(v, i + 24) * Math.PI);
          parts.push(part(g, { tint: 1 - i * 0.06, sway: s.sway }));
        }
        parts.push(part(cone(0.09, h * 0.22, 5, 0, h * 0.9, 0),
          { tint: 0.78, sway: s.sway * 1.2 }));
        return parts;
      }
      case 'palm': {
        // A leaning trunk under a burst of drooping fronds (mirrored quads
        // with up-normals, like blades) and a coconut cluster. The old single
        // sphere read as a lollipop, not a palm.
        const lean = 0.10 + vhash(v, 1) * 0.08;
        const trunk = cyl(0.06, 0.11, h * 0.8, 6);
        trunk.rotateZ(lean);
        const top = { x: -Math.sin(lean) * h * 0.75, y: h * 0.76 };
        const parts = [part(trunk, { fixed: WOOD })];
        const fronds = [];
        const n = 7;
        for (let i = 0; i < n; i++) {
          for (const flip of [0, Math.PI]) {
            const f = new THREE.PlaneGeometry(0.22, 1.15, 1, 3);
            // Droop: bend the frond down along its length.
            const pos = f.attributes.position;
            for (let k = 0; k < pos.count; k++) {
              const yy = pos.getY(k) + 0.575; // 0 at base … 1.15 at tip
              pos.setZ(k, pos.getZ(k) + yy * yy * 0.35); // tips curl down after the tilt
            }
            f.rotateX(Math.PI / 2 - 0.5); // splay outward and down
            f.rotateY((i / n) * Math.PI * 2 + vhash(v, i) * 0.5 + flip);
            f.translate(top.x, top.y, 0);
            fronds.push(f);
          }
        }
        const merged = mergeSame(fronds);
        const normals = merged.attributes.normal;
        for (let k = 0; k < normals.count; k++) normals.setXYZ(k, 0, 1, 0);
        normals.needsUpdate = true;
        parts.push(part(merged, { sway: s.sway * 1.4 }));
        parts.push(part(sphere(0.09, top.x, top.y - 0.12, 0, 6, 4), { fixed: WOOD }));
        parts.push(part(sphere(0.08, top.x + 0.1, top.y - 0.1, 0.06, 6, 4), { fixed: DARKWOOD }));
        return parts;
      }
      case 'acacia': {
        // A forked trunk under layered, offset canopy discs — the fork and
        // the flat top are the acacia's whole read.
        const t1 = cyl(0.07, 0.12, h * 0.58, 6);
        t1.rotateZ(0.16);
        const t2 = cyl(0.06, 0.09, h * 0.5, 6);
        t2.rotateZ(-0.22);
        t2.translate(0.1, h * 0.08, 0.04);
        return [
          part(t1, { fixed: WOOD }),
          part(t2, { fixed: WOOD }),
          part(cyl(0.74, 0.52, h * 0.13, 9, -0.06, h * 0.58, 0), { sway: s.sway }),
          part(cyl(0.5, 0.36, h * 0.10, 8, 0.22 + vhash(v, 2) * 0.1, h * 0.68, 0.1),
            { tint: 1.12, sway: s.sway * 1.2 }),
        ];
      }
      case 'stump':
        return [part(cyl(0.22, 0.26, h, 7), { fixed: WOOD })];
      default: {
        // A broadleaf: tapered trunk with a root flare, and a crown of three
        // to four lumpy lobes on a variant-hashed ring — leaf mass with a
        // silhouette, not one ball on a stick.
        const parts = [
          part(cyl(0.07, 0.13, h * 0.55, 6), { fixed: WOOD }),
          part(cyl(0.16, 0.2, h * 0.06, 6), { fixed: DARKWOOD }),
        ];
        const lobes = 3 + (v % 2);
        for (let i = 0; i < lobes; i++) {
          const a = (i / lobes) * Math.PI * 2 + vhash(v, i) * 1.2;
          const rr = 0.14 + vhash(v, i + 8) * 0.12;
          parts.push(part(
            blob(0.30 + vhash(v, i + 16) * 0.10,
              Math.cos(a) * rr, h * (0.46 + vhash(v, i + 24) * 0.18), Math.sin(a) * rr,
              i % 2),
            { tint: 1 + (i - lobes / 2) * 0.07, sway: s.sway * (1 + i * 0.12) }));
        }
        parts.push(part(blob(0.24, 0, h * 0.72, 0, 0), { tint: 1.14, sway: s.sway * 1.3 }));
        return parts;
      }
    }
  },

  clump(s) {
    switch (s.style) {
      case 'flower':
        return [
          part(blades(0.22, s.h * 0.85, 3), { tint: 1.15, sway: s.sway }),
          part(sphere(0.07, 0, s.h * 0.7, 0, 6, 4), { tint: 1.5, sway: s.sway }),
        ];
      case 'bush':
        return [
          part(blob(s.h * 0.55, 0, 0, 0, 1), { sway: s.sway }),
          part(blob(s.h * 0.4, 0.16, s.h * 0.2, 0.12, 0), { tint: 1.15, sway: s.sway }),
        ];
      case 'reed':
        return [part(blades(0.16, s.h, 3), { sway: s.sway })];
      case 'crop':
        return [
          part(blades(0.5, s.h, 3), { tint: 1.3, sway: s.sway }),
          part(box(0.7, 0.05, 0.7), { tint: 0.7 }),
        ];
      default: // tuft
        // Three crossed blades, narrower than they are tall. Wide, short quads
        // read as a flat "+" painted on the ground from a top-down camera —
        // grass has to stand up to look like grass.
        return [part(blades(s.w * 0.55, s.h, 3), { tint: 1.35, sway: s.sway })];
    }
  },

  rock(s, v = 0) {
    switch (s.style) {
      case 'spire': {
        // A cluster of three sheared prisms at differing tilts and heights —
        // hard rock with a ridgeline, where one cone read as a tent. This is
        // also what mountain peaks wear, so it earns its silhouette.
        const parts = [];
        const spikes = [[0, 0, 1], [-0.22, 0.1, 0.62], [0.2, -0.14, 0.5]];
        for (let i = 0; i < spikes.length; i++) {
          const [sx, sz, sh] = spikes[i];
          const g = cone(s.w * (0.3 - i * 0.05), s.h * sh, 5, sx, 0, sz);
          g.rotateY(vhash(v, i) * Math.PI);
          const tilt = 0.08 + vhash(v, i + 8) * 0.14;
          if (i === 1) g.rotateZ(tilt);
          if (i === 2) g.rotateX(-tilt);
          parts.push(part(g, { tint: 0.95 + i * 0.06 }));
        }
        return parts;
      }
      case 'column': {
        // A speleothem column: a drip-waisted stack, not a plain cylinder.
        return [
          part(cyl(s.w * 0.4, s.w * 0.5, s.h * 0.22, 8), { tint: 0.85 }),
          part(cyl(s.w * 0.22, s.w * 0.34, s.h * 0.56, 8, 0, s.h * 0.22), { tint: 0.92 }),
          part(cyl(s.w * 0.38, s.w * 0.24, s.h * 0.22, 8, 0, s.h * 0.78), { tint: 0.85 }),
        ];
      }
      case 'flowstone':
        // Overlapping drips draping a face, rather than one cone.
        return [
          part(cone(s.w * 0.4, s.h, 7, -0.1, 0, 0.05), { tint: 1.05 }),
          part(cone(s.w * 0.3, s.h * 0.72, 6, 0.14, 0, -0.08), { tint: 1.12 }),
          part(cone(s.w * 0.22, s.h * 0.5, 5, 0.02, 0, 0.16), { tint: 0.98 }),
        ];
      case 'rubble':
        return [
          part(blob(s.h * 0.6, -0.15, 0, 0.1), { tint: 0.95 }),
          part(blob(s.h * 0.45, 0.18, 0, -0.12), { tint: 1.08 }),
        ];
      default:
        return [part(blob(s.h * 0.62, 0, 0, 0, s.style === 'boulder' ? 1 : 0))];
    }
  },

  box(s) {
    const w = s.w, d = s.d, h = s.h;
    switch (s.style) {
      case 'screen':
        return [
          part(box(w, h * 0.12, d, 0, 0), { tint: 0.5 }),
          part(box(w * 0.92, h * 0.7, d * 0.4, 0, h * 0.2), { glow: s.glow, tint: 1.3 }),
        ];
      case 'well':
        return [
          part(cyl(w * 0.5, w * 0.52, h * 0.5, 10), { fixed: STONE }),
          part(box(0.06, h * 0.55, 0.06, -w * 0.35, h * 0.5), { fixed: DARKWOOD }),
          part(box(0.06, h * 0.55, 0.06, w * 0.35, h * 0.5), { fixed: DARKWOOD }),
          part(roof(w, d, h * 0.35, h), { fixed: THATCH }),
        ];
      case 'stall':
        return [
          part(box(w * 0.9, h * 0.5, d * 0.7, 0, 0), { fixed: WOOD }),
          part(roof(w, d, h * 0.4, h * 0.6), { tint: 1.3 }), // the striped awning
        ];
      case 'sign':
        return [
          part(box(0.08, h * 0.5, 0.08, 0, 0), { fixed: DARKWOOD }),
          part(box(w, h * 0.45, 0.06, 0, h * 0.45), { fixed: WOOD }),
        ];
      case 'chest':
        return [
          part(box(w, h * 0.6, d, 0, 0), { fixed: WOOD }),
          part(box(w * 1.02, h * 0.35, d * 1.02, 0, h * 0.6), { tint: 0.8 }),
        ];
      case 'logs':
        return [
          part(cylAxis('x', 0.14, w, 7, 0, 0, -0.15), { fixed: WOOD }),
          part(cylAxis('x', 0.14, w, 7, 0, 0, 0.15), { fixed: DARKWOOD }),
        ];
      case 'furnace':
      case 'sawmill':
      case 'machine':
      case 'turbine':
        return [
          // Machinery is the one thing in the world that should look metallic.
          part(box(w, h * 0.8, d, 0, 0), { tint: 0.9, rough: 0.42, metal: 0.55 }),
          part(box(w * 0.55, h * 0.3, d * 0.55, 0, h * 0.8), { tint: 1.1, rough: 0.42, metal: 0.55 }),
          part(box(w * 0.4, h * 0.22, d * 0.1, 0, h * 0.25, d * 0.5),
            { glow: s.glow, tint: 1.6 }),
        ];
      case 'pipe':
        return [part(cylAxis('x', h * 0.4, w, 8), { tint: 0.95 })];
      case 'workbench':
        return [
          part(box(w, h * 0.15, d, 0, h * 0.6), { fixed: WOOD }),
          part(box(0.08, h * 0.6, 0.08, -w * 0.4, 0, -d * 0.4), { fixed: DARKWOOD }),
          part(box(0.08, h * 0.6, 0.08, w * 0.4, 0, d * 0.4), { fixed: DARKWOOD }),
        ];
      default:
        return [part(box(w, h, d))];
    }
  },

  building(s) {
    const w = s.w * 0.92, d = s.d * 0.92, h = s.h;
    const wallH = h * 0.62, roofH = h * 0.38;
    const parts = [
      part(box(w, wallH, d), { tint: 1 }),
      part(roof(w * 1.06, d * 1.06, roofH, wallH), { tint: 0.62 }),
    ];
    // Lit windows: what makes a village read as inhabited after dark. The
    // server decides which buildings glow (a tavern does, a barn doesn't).
    if (s.glow > 0) {
      parts.push(part(box(w * 0.22, wallH * 0.3, 0.04, 0, wallH * 0.35, d / 2),
        { fixed: GLASS, glow: s.glow }));
    }
    switch (s.style) {
      case 'church':
      case 'cathedral': {
        const spire = h * 0.55;
        parts.push(part(box(w * 0.3, h * 0.75, w * 0.3, -w * 0.28, 0, -d * 0.3), { tint: 0.95 }));
        parts.push(part(cone(w * 0.22, spire, 4, -w * 0.28, h * 0.75, -d * 0.3), { tint: 0.6 }));
        break;
      }
      case 'keep':
        // A keep is crenellated rather than roofed, so it reads as a stronghold.
        parts[1] = part(box(w * 1.04, roofH * 0.35, d * 1.04, 0, wallH), { tint: 0.75 });
        break;
      case 'smithy':
        parts.push(part(cyl(0.12, 0.15, h * 0.4, 6, w * 0.3, wallH, -d * 0.3), { tint: 0.5 }));
        break;
      case 'mill':
        // The windmill's sails: a cross on the front face.
        parts.push(part(box(w * 1.5, 0.12, 0.08, 0, h * 0.75, d * 0.55), { fixed: WOOD }));
        parts.push(part(box(0.12, h * 0.9, 0.08, 0, h * 0.35, d * 0.55), { fixed: WOOD }));
        break;
    }
    return parts;
  },

  fence(s) {
    switch (s.style) {
      case 'post':
        return [part(box(s.w, s.h, s.d), { fixed: WOOD })];
      case 'wall':
        return [part(box(s.w, s.h, s.d), { fixed: STONE })];
      case 'tower':
        return [
          part(cyl(s.w * 0.45, s.w * 0.5, s.h * 0.8, 9), { fixed: STONE }),
          part(cone(s.w * 0.52, s.h * 0.3, 9, 0, s.h * 0.8), { tint: 0.6 }),
        ];
      case 'timber':
        return [
          part(box(0.12, s.h, 0.12, -0.35, 0), { fixed: DARKWOOD }),
          part(box(0.12, s.h, 0.12, 0.35, 0), { fixed: DARKWOOD }),
          part(box(1.0, 0.14, 0.14, 0, s.h - 0.14), { fixed: DARKWOOD }),
        ];
      default: {
        // A rail fence: two horizontal rails between end posts, oriented by style.
        const horiz = s.style === 'h';
        const len = 1.0, thick = 0.08;
        const rail = (y) => horiz
          ? box(len, thick, thick, 0, y)
          : box(thick, thick, len, 0, y);
        return [
          part(rail(s.h * 0.35), { fixed: WOOD }),
          part(rail(s.h * 0.75), { fixed: WOOD }),
          part(box(0.12, s.h, 0.12, horiz ? -0.45 : 0, 0, horiz ? 0 : -0.45), { fixed: DARKWOOD }),
        ];
      }
    }
  },

  flat(s) {
    switch (s.style) {
      case 'chasm':
        // A lit stone rim around a black drop — the hole reads as depth, not as
        // a dark tile, because the rim catches the light and the floor doesn't.
        return [
          part(box(1.0, 0.06, 1.0, 0, -0.02), { fixed: 0x05070a }),
          part(box(1.0, 0.1, 0.14, 0, 0, -0.43), { tint: 0.9 }),
          part(box(1.0, 0.1, 0.14, 0, 0, 0.43), { tint: 0.9 }),
        ];
      case 'pool':
        return [part(box(0.92, s.h, 0.92), { glow: s.glow, tint: 1.3 })];
      case 'bedroll':
        return [part(box(s.w, s.h, s.d), { tint: 1 })];
      default: { // bridges
        const horiz = s.style === 'bridge-h';
        return [
          part(box(1.0, s.h, 0.92), { fixed: WOOD }),
          part(box(horiz ? 1.0 : 0.06, 0.14, horiz ? 0.06 : 1.0,
            0, s.h, horiz ? -0.44 : -0.44), { fixed: DARKWOOD }),
        ];
      }
    }
  },

  glow(s) {
    switch (s.style) {
      case 'lamp':
        return [
          part(cyl(0.05, 0.07, s.h * 0.75, 6), { fixed: DARKWOOD }),
          part(sphere(0.16, 0, s.h * 0.75, 0), { glow: s.glow, tint: 1.6 }),
        ];
      case 'fire':
        return [
          part(cyl(0.2, 0.26, s.h * 0.35, 7), { tint: 0.5 }),
          part(cone(0.18, s.h * 0.7, 6, 0, s.h * 0.3), { glow: s.glow, tint: 1.8 }),
        ];
      case 'orb':
        return [
          part(cyl(0.4, 0.5, s.h * 0.25, 10), { tint: 0.6 }),
          part(sphere(s.h * 0.3, 0, s.h * 0.35, 0, 12, 8), { glow: s.glow, tint: 1.5 }),
        ];
      case 'fountain':
        return [
          part(cyl(0.5, 0.52, s.h * 0.4, 12), { fixed: STONE, rough: 0.75 }),
          part(cyl(0.42, 0.42, s.h * 0.15, 12, 0, s.h * 0.4), { glow: s.glow, tint: 1.6 }),
        ];
      case 'shroom':
        return [
          part(cyl(0.05, 0.07, s.h * 0.5, 6), { tint: 0.8 }),
          part(sphere(0.2, 0, s.h * 0.4, 0, 8, 5), { glow: s.glow, tint: 1.5 }),
        ];
      case 'shaft':
        // A shaft of daylight: a tall, near-transparent cone of light.
        return [part(cone(s.w * 0.5, s.h, 8), { glow: s.glow, tint: 1.8 })];
      case 'geode':
      case 'relic':
      case 'gem':
      default:
        return [
          part(new THREE.OctahedronGeometry(s.h * 0.5).translate(0, s.h * 0.5, 0),
            { glow: s.glow, tint: 1.4 }),
        ];
    }
  },

  portal(s) {
    switch (s.style) {
      case 'cave':
        return [
          part(blob(0.62, 0, 0, -0.1, 1), { tint: 0.75 }),
          part(box(0.5, s.h * 0.6, 0.12, 0, 0, 0.3), { fixed: 0x05070a }),
        ];
      case 'sealed':
        // A broken arch: two uprights and no lintel, so it reads as dormant.
        return [
          part(box(0.16, s.h * 0.8, 0.16, -0.34, 0), { tint: 0.8 }),
          part(box(0.16, s.h * 0.55, 0.16, 0.34, 0), { tint: 0.8 }),
        ];
      default:
        // A live gate: stone jambs, a lintel, and a column of light between them
        // that the renderer pulses. This is the one prop players navigate by, so
        // it is deliberately the tallest thing in a plain field.
        return [
          part(box(0.18, s.h * 0.85, 0.18, -0.36, 0), { tint: 0.7 }),
          part(box(0.18, s.h * 0.85, 0.18, 0.36, 0), { tint: 0.7 }),
          part(box(0.95, 0.18, 0.22, 0, s.h * 0.85), { tint: 0.7 }),
          part(box(0.6, s.h * 0.8, 0.06), { glow: s.glow, tint: 1.6 }),
        ];
    }
  },

  item(s) {
    // Every pickup gets the same ground grammar: a soft dark contact disc
    // (the "something hovers here" anchor that survives any biome color) and
    // a thin accent ring in the palette's find-cyan — readable at full zoom
    // where the trinket itself is four pixels. The trinket bobs above it.
    const ground = [
      part(new THREE.CylinderGeometry(0.2, 0.2, 0.012, 14).translate(0, 0.012, 0),
        { fixed: 0x0a0d12 }),
      part(new THREE.TorusGeometry(0.24, 0.016, 6, 20).rotateX(Math.PI / 2)
        .translate(0, 0.03, 0), { fixed: ACCENT, glow: 0.5 }),
    ];
    if (s.style === 'hat') {
      return [
        ...ground,
        part(cyl(0.24, 0.26, 0.05, 10, 0, 0.14), { tint: 1, bob: true }),
        part(cyl(0.13, 0.15, s.h, 10, 0, 0.19), { tint: 1.2, glow: s.glow, bob: true }),
        part(cyl(0.155, 0.165, 0.05, 10, 0, 0.2), { tint: 0.6, bob: true }),
      ];
    }
    if (s.style === 'fish') {
      return [
        ...ground,
        part(new THREE.OctahedronGeometry(s.h).scale(1.6, 0.7, 0.5)
          .translate(0, s.h * 0.9, 0), { tint: 1, bob: true }),
        // The tail fin: two triangles that make a fish a fish.
        part(new THREE.ConeGeometry(s.h * 0.45, s.h * 0.7, 3)
          .rotateZ(Math.PI / 2).translate(-s.h * 1.7, s.h * 0.9, 0),
          { tint: 0.85, bob: true }),
      ];
    }
    // A gem is a brilliant cut, not a lump: a faceted crown over a pavilion
    // cone, flat-shaded so the facets glint as it bobs.
    const r = s.h * 0.5;
    return [
      ...ground,
      part(new THREE.CylinderGeometry(r * 0.55, r * 0.95, s.h * 0.35, 8)
        .translate(0, s.h * 0.95, 0), { tint: 1.25, glow: s.glow, bob: true }),
      part(new THREE.ConeGeometry(r * 0.95, s.h * 0.6, 8).rotateX(Math.PI)
        .translate(0, s.h * 0.48, 0), { tint: 1.05, glow: s.glow, bob: true }),
    ];
  },

  creature(s) {
    const h = s.h;
    const body = [
      part(new THREE.SphereGeometry(h * 0.42, 8, 6).scale(1.5, 0.85, 1)
        .translate(0, h * 0.5, 0), { tint: 1 }),
      part(sphere(h * 0.26, 0, h * 0.55, h * 0.4, 8, 6), { tint: 1.1 }), // head
    ];
    if (s.style === 'deer') {
      body.push(part(box(0.05, h * 0.3, 0.05, -0.08, h * 0.8, h * 0.4), { fixed: DARKWOOD }));
      body.push(part(box(0.05, h * 0.3, 0.05, 0.08, h * 0.8, h * 0.4), { fixed: DARKWOOD }));
    }
    if (s.style === 'rabbit') {
      body.push(part(box(0.05, h * 0.4, 0.03, -0.06, h * 0.7, h * 0.3), { tint: 1.2 }));
      body.push(part(box(0.05, h * 0.4, 0.03, 0.06, h * 0.7, h * 0.3), { tint: 1.2 }));
    }
    if (s.style === 'bird') {
      body.push(part(coneZ(0.06, 0.14, 5, 0, h * 0.6, h * 0.62), { tint: 1.5 }));
    }
    return body;
  },

  chess(s) {
    const h = s.h;
    const parts = [
      part(cyl(0.16, 0.22, h * 0.25, 10), { tint: 1 }),
      part(cyl(0.1, 0.14, h * 0.5, 10, 0, h * 0.25), { tint: 1 }),
    ];
    switch (s.style) {
      case 'king':
        parts.push(part(sphere(0.11, 0, h * 0.72, 0), { tint: 1 }));
        parts.push(part(box(0.05, 0.16, 0.05, 0, h * 0.9), { tint: 1 }));
        parts.push(part(box(0.14, 0.05, 0.05, 0, h * 0.96), { tint: 1 }));
        break;
      case 'queen':
        parts.push(part(sphere(0.13, 0, h * 0.72, 0), { tint: 1 }));
        parts.push(part(cone(0.09, 0.14, 8, 0, h * 0.88), { tint: 1 }));
        break;
      case 'rook':
        parts.push(part(box(0.26, h * 0.25, 0.26, 0, h * 0.72), { tint: 1 }));
        break;
      case 'bishop':
        parts.push(part(cone(0.12, h * 0.35, 9, 0, h * 0.7), { tint: 1 }));
        break;
      case 'knight':
        parts.push(part(box(0.14, h * 0.3, 0.24, 0, h * 0.7, 0.04), { tint: 1 }));
        break;
      default: // pawn
        parts.push(part(sphere(0.12, 0, h * 0.68, 0), { tint: 1 }));
    }
    return parts;
  },
};

/** A prop the server named but this client doesn't know: a plain marker, in the
 *  prop's own color, so an unmapped prop is visibly unfinished rather than
 *  invisible. */
function fallback() {
  return [part(box(0.5, 0.5, 0.5))];
}

const cache = new Map();

/** partsFor returns (and caches) the geometry parts for a shape key and
 *  variant. Variants give grown things (trees, rocks) a few genuinely
 *  different silhouettes per species — a forest of one mesh, spun, still
 *  reads as one mesh. */
export function partsFor(key, shape, variant = 0) {
  const ck = key + '#' + variant;
  if (cache.has(ck)) return cache.get(ck);
  const build = builders[shape.build];
  let parts;
  try {
    parts = build ? build(shape, variant) : fallback();
  } catch (err) {
    console.warn('durstworld: could not build shape', key, err);
    parts = fallback();
  }
  if (!parts || !parts.length) parts = fallback();
  cache.set(ck, parts);
  return parts;
}
