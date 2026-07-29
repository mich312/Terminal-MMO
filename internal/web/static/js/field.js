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
  F_PROP = 5, F_PCOL = 6, F_FLAGS = 7;
const STRIDE = 8;

const UP = new THREE.Vector3(0, 1, 0);
const WALL_HEIGHT = 1.25;
const WATER_DROP = -0.14; // water sits below the shoreline, so beaches read

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
  constructor(scene, geom, material, cap = 512) {
    this.scene = scene;
    this.geom = geom;
    this.material = material;
    this.cap = cap;
    this.free = [];
    this.next = 0;
    this._mk(cap);
    this._m = new THREE.Matrix4();
    this._zero = new THREE.Matrix4().makeScale(0, 0, 0);
  }

  _mk(cap) {
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

/** Wind. Rather than move 300 trees on the CPU every frame, the sway is a few
 *  lines injected into the vertex shader: displacement grows with height above
 *  the ground and is offset by world position, so neighbouring plants don't
 *  move in lockstep. */
function addWind(material, amount) {
  if (amount <= 0) return material;
  material.userData.windAmount = amount;
  material.onBeforeCompile = (shader) => {
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

export class TileField {
  constructor(scene) {
    this.scene = scene;
    this.palette = [new THREE.Color(0x888888)];
    this.shapes = {};       // shape key → Shape
    this.propKeys = {};     // prop id  → shape key
    this.texNames = {};     // tex id   → surface name
    this.tiles = new Map(); // "x,y" → { ground, wall, props:[{pool,id}] }
    this.propPools = new Map(); // shape key → [InstancePool per part]
    this._m = new THREE.Matrix4();
    this._q = new THREE.Quaternion();
    this._p = new THREE.Vector3();
    this._s = new THREE.Vector3(1, 1, 1);
    this._c = new THREE.Color();
    this.windMaterials = [];

    // Ground is pooled by *surface class*, not by biome: the server already
    // says whether a tile is grass, rock, metal or water, and that is exactly
    // the information a PBR material needs. Snow and sand share a roughness;
    // a machine-hall floor does not. Four pools cover fourteen textures.
    this.groundPlane = new THREE.PlaneGeometry(1, 1).rotateX(-Math.PI / 2);
    this.groundPools = new Map();

    const cube = new THREE.BoxGeometry(1, WALL_HEIGHT, 1).translate(0, WALL_HEIGHT / 2, 0);
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
    pool = new InstancePool(this.scene, this.groundPlane.clone(), material,
      cls === 'soft' ? 2048 : 512);
    // The ground receives shadows but never casts one: it's a flat plane, so a
    // cast shadow would only ever be self-shadowing acne.
    pool.setShadows(false, true);
    this.groundPools.set(cls, pool);
    return pool;
  }

  /** setVocabulary takes the shape table from the server's hello message. */
  setVocabulary(shapes, propKeys, texNames) {
    this.shapes = shapes || {};
    this.propKeys = propKeys || {};
    this.texNames = texNames || {};
  }

  /** apply folds one scene message into the live field. */
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
      for (let i = 0; i < msg.tiles.length; i += STRIDE) {
        this.setTile(msg.tiles, i);
      }
    }
  }

  color(idx) {
    return this.palette[idx] || this.palette[0];
  }

  clearAll() {
    this.tiles.clear();
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
  }

  setTile(arr, i) {
    const x = arr[i + F_X], y = arr[i + F_Y];
    const key = x + ',' + y;
    this.removeTile(key); // a changed tile is rebuilt, not patched: tiles change
    // rarely and a rebuild is far simpler to get right than a diff

    const kind = arr[i + F_KIND];
    if (kind === KIND_VOID) return; // outside the map: leave a hole

    const tex = this.texNames[arr[i + F_TEX]] || 'flat';
    const cls = surfaceClass(tex);
    const isWater = cls === 'water';
    const rec = { props: [] };

    // Ground.
    const gc = this.color(arr[i + F_GROUND]);
    const pool = this.groundPoolFor(cls);
    this._p.set(x + 0.5, isWater ? WATER_DROP : 0, y + 0.5);
    this._s.set(1, 1, 1);
    this._m.compose(this._p, this._q.identity(), this._s);
    // A whisper of per-tile brightness variation. A field of one exact green is
    // the other half of why generated ground reads as tiled; ±3% is invisible
    // as an effect and very visible as an absence.
    this._c.copy(gc).multiplyScalar(0.97 + hash2(x, y, 7) * 0.06);
    rec.ground = { pool, id: pool.alloc(this._m, this._c) };

    // Walls are extruded from the same color as their tile, slightly darkened
    // so the top face and the sides don't merge into one flat block.
    if (kind === KIND_WALL) {
      this._p.set(x + 0.5, 0, y + 0.5);
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
        this.placeProp(rec, shapeKey, shape, x, y, pc);
      }
    }
    this.tiles.set(key, rec);
  }

  /** poolsFor lazily builds one InstancePool per part of a shape. */
  poolsFor(shapeKey, shape) {
    let pools = this.propPools.get(shapeKey);
    if (pools) return pools;
    const parts = partsFor(shapeKey, shape);
    pools = parts.map((p) => {
      // Glowing parts stay unlit. A standard material's `emissive` is a single
      // material-wide color, but the *instance* carries the hue here — every
      // campfire, gem and portal in one pool shares a material and differs only
      // by instance color, so an emissive material would light them all the
      // same wrong color. Unlit keeps each one its own hue, and because basic
      // materials are tone-mapped too it still sits inside the graded image
      // rather than punching a flat hole in it.
      const material = p.glow > 0
        ? new THREE.MeshBasicMaterial({ color: 0xffffff })
        : new THREE.MeshStandardMaterial({
          color: 0xffffff,
          roughness: p.rough ?? 0.88,
          metalness: p.metal ?? 0,
        });
      if (p.double) material.side = THREE.DoubleSide;
      if (p.sway > 0) {
        addWind(material, p.sway);
        this.windMaterials.push(material);
      }
      // Foliage is drawn double-sided with faked-up normals, so it makes a poor
      // shadow caster (it self-shadows into mush); everything solid casts.
      const pool = new InstancePool(this.scene, p.geom, material, 128)
        .setShadows(!p.double && p.glow === 0, !p.double);
      pool.part = p;
      return pool;
    });
    this.propPools.set(shapeKey, pools);
    return pools;
  }

  placeProp(rec, shapeKey, shape, x, y, color) {
    const pools = this.poolsFor(shapeKey, shape);
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
    let scale = 1, spin = 0, shade = 1;
    if (jitter > 0) {
      spin = hash2(x, y, 1) * Math.PI * 2;
      scale = 1 + (hash2(x, y, 2) - 0.5) * 2 * jitter;
      // Vary the tone as well as the shape — a stand of trees is never one green.
      shade = 1 + (hash2(x, y, 3) - 0.5) * 0.22;
    }
    this._q.setFromAxisAngle(UP, spin);

    for (const pool of pools) {
      const p = pool.part;
      this._p.set(x + 0.5 + offX, 0, y + 0.5 + offZ);
      this._m.compose(this._p, this._q, this._s.set(scale, scale, scale));
      if (p.fixed != null) {
        this._c.setHex(p.fixed).multiplyScalar(shade);
      } else {
        this._c.copy(color).multiplyScalar(p.tint * shade);
      }
      rec.props.push({ pool, id: pool.alloc(this._m, this._c) });
    }
  }

  /** update advances the wind clock. */
  update(time) {
    for (const m of this.windMaterials) {
      const sh = m.userData.shader;
      if (sh) sh.uniforms.uTime.value = time;
    }
  }
}
