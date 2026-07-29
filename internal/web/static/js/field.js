/* The tile field: ground, walls and props, all instanced.
 *
 * The server addresses tiles by absolute world coordinate and sends only what
 * changed, so this module's job is to keep a live scene in sync with a stream
 * of "tile (x,y) is now this" and "forget tile (x,y)". Walking a step usually
 * costs one row.
 *
 * Everything is drawn with InstancedMesh, pooled per material. A screen of
 * forest is a handful of draw calls rather than a thousand, which is what lets
 * the client hold a 64×48 window of a generative overworld at 60fps on a
 * laptop's integrated GPU.
 */

import * as THREE from 'three';
import { partsFor } from './props.js';

/* Tile kinds, mirroring internal/game/tilemap.go. */
const KIND_VOID = 0, KIND_WALL = 2;

/* Field offsets within the server's flat tile array (protocol.go TileStride). */
const F_X = 0, F_Y = 1, F_KIND = 2, F_TEX = 3, F_GROUND = 4,
  F_PROP = 5, F_PCOL = 6, F_FLAGS = 7, F_ELEV = 8;
const STRIDE = 9;

const UP = new THREE.Vector3(0, 1, 0);
const WALL_HEIGHT = 1.25;
const WALL_SKIRT = 1.0;   // walls extend below grade, so a cube on a slope has no gap
const WATER_LEVEL = -0.14; // the one flat sea level; shores ramp down to meet it

/* The elevation curve: the server ships raw surface elevation (0…255 ≈ 0…1,
   sea level at ~0.34) and this shapes it into world units. Below sea level
   drops away fast (lake beds); above it the lowland rolls gently and the
   highlands rise hard, so hills read as hills without turning a meadow walk
   into mountaineering. Zero is the "flat area" sentinel — hand-built rooms
   send no elevation and must sit exactly at grade (real terrain never sends
   0: even the deep ocean floor is above it). */
function elevToY(e255) {
  if (!e255) return 0;
  const t = e255 / 255 - 0.34;
  if (t <= 0) return t * 3.0;
  return t * 1.1 + t * t * 4.6;
}

/* Surface classes. The server sends a texture per tile (grass, brick, metal…);
   these are the physical properties that texture implies. It is the cheapest
   possible material system and it costs no new data at all. */
const SURFACES = {
  soft: { roughness: 0.97, metalness: 0.0 },   // grass, sand, snow, dirt, fields
  stone: { roughness: 0.78, metalness: 0.0 },  // rock, brick, interior floors
  metal: { roughness: 0.38, metalness: 0.55 }, // machine halls
  water: { roughness: 0.08, metalness: 0.35, envMapIntensity: 2.2 },
};

/** surfaceClass maps a texture name to how that material behaves in light. */
function surfaceClass(tex) {
  switch (tex) {
    case 'water': return 'water';
    case 'metal': return 'metal';
    case 'rock': case 'brick': case 'floor': return 'stone';
    default: return 'soft';
  }
}

/** hash2 is a stable 2D value hash. Instance variation is seeded from a tile's
 *  own coordinates rather than from a random number, so the same tree is the
 *  same tree for every player, on every reconnect, forever — the overworld is
 *  deterministic and its rendering has to be too. */
function hash2(x, y, salt = 0) {
  let h = Math.imul(x | 0, 374761393) ^ Math.imul(y | 0, 668265263) ^ Math.imul(salt, 2246822519);
  h = Math.imul(h ^ (h >>> 13), 1274126177);
  return ((h ^ (h >>> 16)) >>> 0) / 4294967296;
}

/** InstancePool is one InstancedMesh plus a free list, so instances can be
 *  handed out and returned as tiles come and go. A returned slot is collapsed
 *  to zero scale rather than compacted — compaction would invalidate every id
 *  we've handed out, and the slot is reused on the next allocation anyway. */
class InstancePool {
  constructor(scene, geom, material, cap = 512, corners = false) {
    this.scene = scene;
    this.geom = geom;
    this.material = material;
    this.cap = cap;
    this.corners = corners; // ground pools carry per-instance corner heights
    this.free = [];
    this.next = 0;
    this._mk(cap);
    this._m = new THREE.Matrix4();
    this._zero = new THREE.Matrix4().makeScale(0, 0, 0);
  }

  _mk(cap) {
    if (this.corners) {
      // The corner-height attribute lives on the geometry, so growing swaps in
      // a bigger array and keeps what's already there.
      const arr = new Float32Array(cap * 4);
      const old = this.geom.getAttribute('aCorner');
      if (old) arr.set(old.array);
      const attr = new THREE.InstancedBufferAttribute(arr, 4);
      attr.setUsage(THREE.DynamicDrawUsage);
      this.geom.setAttribute('aCorner', attr);
    }
    const mesh = new THREE.InstancedMesh(this.geom, this.material, cap);
    mesh.instanceMatrix.setUsage(THREE.DynamicDrawUsage);
    mesh.count = 0;
    mesh.frustumCulled = false; // instances span the whole window; culling the
    // mesh as a unit would pop the entire field
    // Growing a pool builds a new mesh, so the shadow flags have to live on the
    // pool rather than on the mesh — otherwise a forest silently stops casting
    // shadows the moment it outgrows its first buffer.
    mesh.castShadow = this.doesCast === true;
    mesh.receiveShadow = this.doesReceive === true;
    mesh.customDepthMaterial = this.depthMaterial || undefined;
    if (this.mesh) {
      // Growing: copy the old instance data into the bigger buffer.
      mesh.count = this.mesh.count;
      mesh.instanceMatrix.array.set(this.mesh.instanceMatrix.array);
      if (this.mesh.instanceColor) {
        mesh.instanceColor = new THREE.InstancedBufferAttribute(
          new Float32Array(cap * 3), 3);
        mesh.instanceColor.array.set(this.mesh.instanceColor.array);
        mesh.instanceColor.needsUpdate = true;
      }
      this.scene.remove(this.mesh);
      this.mesh.dispose();
    }
    this.scene.add(mesh);
    this.mesh = mesh;
    this.cap = cap;
  }

  /** setDepthMaterial installs a custom shadow-pass material (needed whenever
   *  the main material displaces vertices, or the shadow won't match). */
  setDepthMaterial(m) {
    this.depthMaterial = m;
    this.mesh.customDepthMaterial = m;
    return this;
  }

  /** setCorner writes one instance's four corner heights (ground pools only). */
  setCorner(id, c00, c10, c01, c11) {
    const attr = this.geom.getAttribute('aCorner');
    if (!attr) return;
    attr.setXYZW(id, c00, c10, c01, c11);
    attr.needsUpdate = true;
  }

  /** setShadows records whether this pool casts and receives, and applies it —
   *  including across every future growth. */
  setShadows(cast, receive) {
    this.doesCast = cast;
    this.doesReceive = receive;
    this.mesh.castShadow = cast;
    this.mesh.receiveShadow = receive;
    return this;
  }

  alloc(matrix, color) {
    let id;
    if (this.free.length) {
      id = this.free.pop();
    } else {
      if (this.next >= this.cap) this._mk(this.cap * 2);
      id = this.next++;
      this.mesh.count = this.next;
    }
    this.mesh.setMatrixAt(id, matrix);
    if (color) this.mesh.setColorAt(id, color);
    this.mesh.instanceMatrix.needsUpdate = true;
    if (this.mesh.instanceColor) this.mesh.instanceColor.needsUpdate = true;
    return id;
  }

  release(id) {
    if (id == null) return;
    this.mesh.setMatrixAt(id, this._zero);
    this.mesh.instanceMatrix.needsUpdate = true;
    this.free.push(id);
  }

  clear() {
    this.free.length = 0;
    this.next = 0;
    this.mesh.count = 0;
  }
}

/* The terrain displacement, as GLSL chunks shared by the ground's render and
   shadow materials. Each ground instance carries its four corner heights; the
   plane's vertices lerp between them bilinearly, and the normal is rebuilt
   from the corner deltas so a slope actually catches the light. */
const CORNER_VERTEX = `
  float hu = clamp(position.x + 0.5, 0.0, 1.0);
  float hv = clamp(position.z + 0.5, 0.0, 1.0);
  transformed.y += mix(mix(aCorner.x, aCorner.y, hu), mix(aCorner.z, aCorner.w, hu), hv);`;
const CORNER_NORMAL = `
  {
    float hu = clamp(position.x + 0.5, 0.0, 1.0);
    float hv = clamp(position.z + 0.5, 0.0, 1.0);
    float dhdx = mix(aCorner.y - aCorner.x, aCorner.w - aCorner.z, hv);
    float dhdz = mix(aCorner.z - aCorner.x, aCorner.w - aCorner.y, hu);
    objectNormal = normalize(vec3(-dhdx, 1.0, -dhdz));
  }`;

/** addTerrain injects the corner-height displacement into a ground material.
 *  Water additionally ripples: two crossing sines on world position and time —
 *  the terminal's waterGlint, translated into motion. */
function addTerrain(material, ripple, windMaterials) {
  material.onBeforeCompile = (shader) => {
    shader.uniforms.uTime = { value: 0 };
    material.userData.shader = shader;
    shader.vertexShader = 'uniform float uTime;\nattribute vec4 aCorner;\n' +
      shader.vertexShader
        .replace('#include <begin_vertex>',
          `#include <begin_vertex>\n${CORNER_VERTEX}` + (ripple ? `
           #ifdef USE_INSTANCING
             float wwx = instanceMatrix[3].x + position.x;
             float wwz = instanceMatrix[3].z + position.z;
           #else
             float wwx = position.x; float wwz = position.z;
           #endif
           transformed.y += sin(wwx * 1.7 + uTime * 1.2) * 0.02
                          + cos(wwz * 2.3 + uTime * 0.9) * 0.015;` : ''))
        .replace('#include <beginnormal_vertex>',
          `#include <beginnormal_vertex>\n${CORNER_NORMAL}`);
  };
  if (ripple) windMaterials.push(material); // ride the shared wind clock
  return material;
}

/** terrainDepthMaterial builds the matching shadow-pass material, so displaced
 *  ground casts a displaced shadow rather than its old flat one. */
function terrainDepthMaterial() {
  const m = new THREE.MeshDepthMaterial({ depthPacking: THREE.RGBADepthPacking });
  m.onBeforeCompile = (shader) => {
    shader.vertexShader = 'attribute vec4 aCorner;\n' + shader.vertexShader
      .replace('#include <begin_vertex>', `#include <begin_vertex>\n${CORNER_VERTEX}`);
  };
  return m;
}

/** Wind. Rather than move 300 trees on the CPU every frame, the sway is a few
 *  lines injected into the vertex shader: displacement grows with height above
 *  the ground and is offset by world position, so neighbouring plants don't
 *  move in lockstep. */
function addWind(material, amount) {
  if (amount <= 0) return material;
  material.userData.windAmount = amount;
  const prev = material.onBeforeCompile;
  material.onBeforeCompile = (shader) => {
    if (prev) prev(shader);
    shader.uniforms.uTime = { value: 0 };
    shader.uniforms.uWind = { value: amount };
    material.userData.shader = shader;
    shader.vertexShader = 'uniform float uTime;\nuniform float uWind;\n' +
      shader.vertexShader.replace(
        '#include <begin_vertex>',
        `#include <begin_vertex>
         #ifdef USE_INSTANCING
           float wx = instanceMatrix[3].x;
           float wz = instanceMatrix[3].z;
         #else
           float wx = 0.0; float wz = 0.0;
         #endif
         float phase = wx * 0.7 + wz * 0.9;
         float lean = uWind * 0.06 * max(transformed.y, 0.0);
         transformed.x += sin(uTime * 1.6 + phase) * lean;
         transformed.z += cos(uTime * 1.1 + phase * 1.3) * lean * 0.6;`);
  };
  return material;
}

/** Hover. Pickups float and bob gently over their tile — motion is what makes
 *  a gem read as loot from across a field, and it costs nothing: the same
 *  clock the wind runs on, phase-shifted per instance. */
function addBob(material) {
  const prev = material.onBeforeCompile;
  material.onBeforeCompile = (shader) => {
    if (prev) prev(shader);
    shader.uniforms.uTime = { value: 0 };
    material.userData.shader = shader;
    shader.vertexShader = 'uniform float uTime;\n' +
      shader.vertexShader.replace(
        '#include <begin_vertex>',
        `#include <begin_vertex>
         #ifdef USE_INSTANCING
           float bphase = instanceMatrix[3].x * 1.3 + instanceMatrix[3].z * 2.1;
         #else
           float bphase = 0.0;
         #endif
         transformed.y += (sin(uTime * 2.2 + bphase) * 0.5 + 0.5) * 0.09 + 0.03;`);
  };
  return material;
}

export class TileField {
  constructor(scene) {
    this.scene = scene;
    this.palette = [new THREE.Color(0x888888)];
    this.shapes = {};       // shape key → Shape
    this.propKeys = {};     // prop id  → shape key
    this.texNames = {};     // tex id   → surface name
    this.tiles = new Map(); // "x,y" → { ground, wall, props:[{pool,id}], data }
    this.heights = new Map(); // "x,y" → shaped elevation (world y), for corners
    this.propPools = new Map(); // shape key#variant → [InstancePool per part]
    this._m = new THREE.Matrix4();
    this._q = new THREE.Quaternion();
    this._p = new THREE.Vector3();
    this._s = new THREE.Vector3(1, 1, 1);
    this._c = new THREE.Color();
    this.windMaterials = [];
    this.glowMaterials = []; // emissive pools, re-scaled by the night clock
    this._night = 0;

    // Ground is pooled by *surface class*, not by biome: the server already
    // says whether a tile is grass, rock, metal or water, and that is exactly
    // the information a PBR material needs. Snow and sand share a roughness;
    // a machine-hall floor does not. Four pools cover fourteen textures.
    // 2×2 segments, so the corner-height displacement bends smoothly rather
    // than creasing along one diagonal.
    this.groundPlane = new THREE.PlaneGeometry(1, 1, 2, 2).rotateX(-Math.PI / 2);
    this.groundPools = new Map();

    // Walls carry a skirt below grade: on a slope the cube's downhill face
    // would otherwise hang open above the neighbouring, lower ground.
    const cube = new THREE.BoxGeometry(1, WALL_HEIGHT + WALL_SKIRT, 1)
      .translate(0, (WALL_HEIGHT - WALL_SKIRT) / 2, 0);
    this.wallPool = new InstancePool(scene, cube,
      new THREE.MeshStandardMaterial({ color: 0xffffff, roughness: 0.82, metalness: 0.02 }),
      1024).setShadows(true, true);
  }

  /** groundPoolFor lazily makes one pool per surface class. */
  groundPoolFor(cls) {
    let pool = this.groundPools.get(cls);
    if (pool) return pool;
    const spec = SURFACES[cls] || SURFACES.soft;
    const material = new THREE.MeshStandardMaterial({
      color: 0xffffff,
      roughness: spec.roughness,
      metalness: spec.metalness,
      // Water is the one surface that should catch the sky.
      envMapIntensity: spec.envMapIntensity ?? 1,
    });
    addTerrain(material, cls === 'water', this.windMaterials);
    pool = new InstancePool(this.scene, this.groundPlane.clone(), material,
      cls === 'soft' ? 2048 : 512, true);
    // Displaced ground both receives and casts: a hill without a cast shadow
    // reads as painted scenery. The matching depth material keeps the shadow
    // in step with the displacement.
    pool.setShadows(cls !== 'water', true);
    pool.setDepthMaterial(terrainDepthMaterial());
    this.groundPools.set(cls, pool);
    return pool;
  }

  /** cornerY is the terrain height at a tile corner — the mean of the shaped
   *  heights of the up-to-4 tiles that meet there. Every tile computes its
   *  shared corners from the same inputs, so adjacent instances (even from
   *  different pools) meet exactly, with no welded mesh needed. */
  cornerY(cx, cy) {
    let sum = 0, n = 0;
    for (let dy = -1; dy <= 0; dy++) {
      for (let dx = -1; dx <= 0; dx++) {
        const h = this.heights.get((cx + dx) + ',' + (cy + dy));
        if (h !== undefined) { sum += h; n++; }
      }
    }
    return n ? sum / n : 0;
  }

  /** heightAt samples the walkable terrain surface at a world position, for
   *  actors and the camera — bilinear over the tile's four corner heights. */
  heightAt(x, z) {
    const tx = Math.floor(x), tz = Math.floor(z);
    const u = x - tx, v = z - tz;
    const c00 = this.cornerY(tx, tz), c10 = this.cornerY(tx + 1, tz);
    const c01 = this.cornerY(tx, tz + 1), c11 = this.cornerY(tx + 1, tz + 1);
    return (c00 * (1 - u) + c10 * u) * (1 - v) + (c01 * (1 - u) + c11 * u) * v;
  }

  /** setVocabulary takes the shape table from the server's hello message. */
  setVocabulary(shapes, propKeys, texNames) {
    this.shapes = shapes || {};
    this.propKeys = propKeys || {};
    this.texNames = texNames || {};
  }

  /** apply folds one scene message into the live field. Heights land first —
   *  a tile's corners average its neighbours', so every height in the batch
   *  must be known before any tile is built — and afterwards the previously
   *  built neighbours of changed tiles are rebuilt, because their shared
   *  corners just moved. */
  apply(msg) {
    if (msg.reset) this.clearAll();
    if (msg.palAdd) for (const hex of msg.palAdd) {
      this.palette.push(new THREE.Color(hex || '#888888'));
    }
    if (msg.drop) {
      for (let i = 0; i < msg.drop.length; i += 2) {
        this.removeTile(msg.drop[i] + ',' + msg.drop[i + 1]);
      }
    }
    if (msg.tiles) {
      const touched = new Set();
      for (let i = 0; i < msg.tiles.length; i += STRIDE) {
        const x = msg.tiles[i + F_X], y = msg.tiles[i + F_Y];
        const key = x + ',' + y;
        if (msg.tiles[i + F_KIND] === KIND_VOID) this.heights.delete(key);
        else this.heights.set(key, elevToY(msg.tiles[i + F_ELEV] || 0));
        touched.add(key);
      }
      for (let i = 0; i < msg.tiles.length; i += STRIDE) {
        this.setTile(msg.tiles, i);
      }
      this.refreshAround(touched);
    }
  }

  /** refreshAround rebuilds the already-built neighbours of freshly changed
   *  tiles, so their corner heights follow. A step costs roughly one extra
   *  row of rebuilt tiles — cheap against the pooled instancing. */
  refreshAround(touched) {
    const redo = new Set();
    for (const key of touched) {
      const [x, y] = key.split(',').map(Number);
      for (let dy = -1; dy <= 1; dy++) {
        for (let dx = -1; dx <= 1; dx++) {
          const k = (x + dx) + ',' + (y + dy);
          if (!touched.has(k) && this.tiles.has(k)) redo.add(k);
        }
      }
    }
    for (const k of redo) {
      const rec = this.tiles.get(k);
      if (rec && rec.data) this.setTile(rec.data, 0);
    }
  }

  color(idx) {
    return this.palette[idx] || this.palette[0];
  }

  clearAll() {
    this.tiles.clear();
    this.heights.clear();
    for (const pool of this.groundPools.values()) pool.clear();
    this.wallPool.clear();
    for (const pools of this.propPools.values()) for (const p of pools) p.clear();
  }

  removeTile(key) {
    const t = this.tiles.get(key);
    if (!t) return;
    if (t.ground) t.ground.pool.release(t.ground.id);
    if (t.wall != null) this.wallPool.release(t.wall);
    if (t.props) for (const p of t.props) p.pool.release(p.id);
    this.tiles.delete(key);
    this.heights.delete(key);
  }

  setTile(arr, i) {
    const x = arr[i + F_X], y = arr[i + F_Y];
    const key = x + ',' + y;
    const keepH = this.heights.get(key); // removeTile clears the height too
    this.removeTile(key); // a changed tile is rebuilt, not patched: tiles change
    // rarely and a rebuild is far simpler to get right than a diff

    const kind = arr[i + F_KIND];
    if (kind === KIND_VOID) return; // outside the map: leave a hole
    if (keepH !== undefined) this.heights.set(key, keepH);

    const tex = this.texNames[arr[i + F_TEX]] || 'flat';
    const cls = surfaceClass(tex);
    const isWater = cls === 'water';
    // Keep the raw stride so a neighbour's height change can rebuild this tile.
    const rec = { props: [], data: arr.slice(i, i + STRIDE) };

    // The tile's terrain: water is one flat sheet at sea level (a lake is
    // level by definition — the shore ramps down to meet it); land lifts its
    // four corners to the heights shared with its neighbours.
    let c00 = 0, c10 = 0, c01 = 0, c11 = 0, baseY = 0;
    if (isWater) {
      baseY = WATER_LEVEL;
    } else {
      c00 = this.cornerY(x, y);
      c10 = this.cornerY(x + 1, y);
      c01 = this.cornerY(x, y + 1);
      c11 = this.cornerY(x + 1, y + 1);
    }
    const centerY = isWater ? WATER_LEVEL : (c00 + c10 + c01 + c11) / 4;

    // Ground.
    const gc = this.color(arr[i + F_GROUND]);
    const pool = this.groundPoolFor(cls);
    this._p.set(x + 0.5, baseY, y + 0.5);
    this._s.set(1, 1, 1);
    this._m.compose(this._p, this._q.identity(), this._s);
    // A whisper of per-tile brightness variation. A field of one exact green is
    // the other half of why generated ground reads as tiled; ±3% is invisible
    // as an effect and very visible as an absence.
    this._c.copy(gc).multiplyScalar(0.97 + hash2(x, y, 7) * 0.06);
    rec.ground = { pool, id: pool.alloc(this._m, this._c) };
    pool.setCorner(rec.ground.id, c00, c10, c01, c11);

    // Walls are extruded from the same color as their tile, slightly darkened
    // so the top face and the sides don't merge into one flat block. They rise
    // from the tile's terrain height — a mountain wall stands on its hill.
    if (kind === KIND_WALL) {
      this._p.set(x + 0.5, centerY, y + 0.5);
      this._m.compose(this._p, this._q.identity(), this._s.set(1, 1, 1));
      this._c.copy(gc).multiplyScalar(0.92);
      rec.wall = this.wallPool.alloc(this._m, this._c);
    }

    // Prop.
    const propID = arr[i + F_PROP];
    if (propID) {
      const shapeKey = this.propKeys[propID];
      const shape = shapeKey && this.shapes[shapeKey];
      if (shape) {
        const pc = this.color(arr[i + F_PCOL]);
        this.placeProp(rec, shapeKey, shape, x, y, pc, centerY);
      }
    }
    this.tiles.set(key, rec);
  }

  /** poolsFor lazily builds one InstancePool per part of a shape variant. */
  poolsFor(shapeKey, shape, variant) {
    const poolKey = shapeKey + '#' + variant;
    let pools = this.propPools.get(poolKey);
    if (pools) return pools;
    const parts = partsFor(shapeKey, shape, variant);
    pools = parts.map((p) => {
      // Glowing parts are lit-but-emissive. The old unlit MeshBasicMaterial
      // discarded the glow *strength* (a 0.15 gem rendered like a 0.9 portal)
      // and, worse, sat below the exposure of sunlit ground — loot read as a
      // dark hole at noon. Emissive keeps the shading, honors the strength,
      // and the per-instance hue survives because the fragment's vColor is
      // multiplied into the emission below.
      const material = new THREE.MeshStandardMaterial({
        color: 0xffffff,
        roughness: p.rough ?? (p.glow > 0 ? 0.55 : 0.88),
        metalness: p.metal ?? 0,
      });
      if (p.glow > 0) {
        material.emissive.setRGB(1, 1, 1);
        material.emissiveIntensity = p.glow;
        const prev = material.onBeforeCompile;
        material.onBeforeCompile = (shader) => {
          if (prev) prev(shader);
          shader.fragmentShader = shader.fragmentShader.replace(
            '#include <emissivemap_fragment>',
            `#include <emissivemap_fragment>
             #ifdef USE_INSTANCING_COLOR
               totalEmissiveRadiance *= vColor;
             #endif`);
        };
        material.userData.glow = p.glow;
        this.glowMaterials.push(material);
        this._applyNight(material);
      }
      if (p.double) material.side = THREE.DoubleSide;
      if (p.sway > 0) {
        addWind(material, p.sway);
        this.windMaterials.push(material);
      }
      if (p.bob) {
        addBob(material);
        this.windMaterials.push(material); // shares the wind clock
      }
      // Foliage is drawn double-sided with faked-up normals, so it makes a poor
      // shadow caster (it self-shadows into mush); everything solid casts.
      const pool = new InstancePool(this.scene, p.geom, material, 128)
        .setShadows(!p.double && p.glow === 0, !p.double);
      pool.part = p;
      return pool;
    });
    this.propPools.set(poolKey, pools);
    return pools;
  }

  placeProp(rec, shapeKey, shape, x, y, color, groundY = 0) {
    // Grown things pick one of a few geometry variants by their own tile hash,
    // so a forest holds several distinct trees instead of one mesh spun about.
    const variant = shape.jitter > 0 ? Math.floor(hash2(x, y, 11) * 3) : 0;
    const pools = this.poolsFor(shapeKey, shape, variant);
    // Multi-tile buildings are anchored bottom-left and overhang up and right,
    // matching how the tileset draws them, so a 3×4 cathedral covers the same
    // ground in both clients.
    const offX = (shape.w > 1 ? (shape.w - 1) / 2 : 0);
    const offZ = (shape.d > 1 ? -(shape.d - 1) / 2 : 0);

    // Per-instance variation, for the things the server says grew rather than
    // were built. Identical, grid-aligned copies are the loudest tell that a
    // world was generated; a turn and a few percent of size breaks it, and
    // because it's hashed from the tile's coordinates it's stable forever.
    const jitter = shape.jitter || 0;
    let scale = 1, tall = 1, spin = 0, shade = 1;
    if (jitter > 0) {
      spin = hash2(x, y, 1) * Math.PI * 2;
      scale = 1 + (hash2(x, y, 2) - 0.5) * 2 * jitter;
      // Height varies more than girth — a stand where every tree is exactly
      // one height is the loudest generated-world tell after grid alignment.
      tall = 1 + (hash2(x, y, 5) - 0.5) * 2 * jitter * 1.6;
      // Vary the tone as well as the shape — a stand of trees is never one green.
      shade = 1 + (hash2(x, y, 3) - 0.5) * 0.22;
    }
    this._q.setFromAxisAngle(UP, spin);

    // Props sit a whisker below the tile's centre height, so a trunk on a
    // slope stays planted instead of hovering at its downhill edge.
    const py = groundY - (jitter > 0 ? 0.04 : 0);
    for (const pool of pools) {
      const p = pool.part;
      this._p.set(x + 0.5 + offX, py, y + 0.5 + offZ);
      this._m.compose(this._p, this._q, this._s.set(scale, scale * tall, scale));
      if (p.fixed != null) {
        this._c.setHex(p.fixed).multiplyScalar(shade);
      } else {
        this._c.copy(color).multiplyScalar(p.tint * shade);
      }
      rec.props.push({ pool, id: pool.alloc(this._m, this._c) });
    }
  }

  /** setNight scales the glow pools with the clock, matching the terminal's
   *  emitters: a beacon in the dark, a glint by day. */
  setNight(n) {
    const night = Math.max(0, Math.min(1, n || 0));
    if (this._night === night) return;
    this._night = night;
    for (const m of this.glowMaterials) this._applyNight(m);
  }

  _applyNight(material) {
    material.emissiveIntensity = material.userData.glow * (0.4 + 1.1 * this._night);
  }

  /** update advances the wind clock. */
  update(time) {
    for (const m of this.windMaterials) {
      const sh = m.userData.shader;
      if (sh) sh.uniforms.uTime.value = time;
    }
  }
}
