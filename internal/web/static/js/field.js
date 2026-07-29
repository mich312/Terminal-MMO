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

const WALL_HEIGHT = 1.25;
const WATER_DROP = -0.14; // water sits below the shoreline, so beaches read

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
    mesh.castShadow = false;
    mesh.receiveShadow = false;
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

    const plane = new THREE.PlaneGeometry(1, 1).rotateX(-Math.PI / 2);
    this.groundPool = new InstancePool(scene, plane,
      new THREE.MeshLambertMaterial({ color: 0xffffff }), 2048);
    // Water gets its own pool so it can be shinier and sit lower than the land
    // around it — a shoreline you can actually see from a 3/4 view.
    this.waterPool = new InstancePool(scene, plane.clone(),
      new THREE.MeshPhongMaterial({ color: 0xffffff, shininess: 90, specular: 0x335577 }),
      512);
    const cube = new THREE.BoxGeometry(1, WALL_HEIGHT, 1).translate(0, WALL_HEIGHT / 2, 0);
    this.wallPool = new InstancePool(scene, cube,
      new THREE.MeshLambertMaterial({ color: 0xffffff }), 1024);
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
    this.groundPool.clear();
    this.waterPool.clear();
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
    const isWater = tex === 'water';
    const rec = { props: [] };

    // Ground.
    const gc = this.color(arr[i + F_GROUND]);
    const pool = isWater ? this.waterPool : this.groundPool;
    this._p.set(x + 0.5, isWater ? WATER_DROP : 0, y + 0.5);
    this._s.set(1, 1, 1);
    this._m.compose(this._p, this._q.identity(), this._s);
    rec.ground = { pool, id: pool.alloc(this._m, gc) };

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
      // A glowing part is drawn unlit, so it stays bright when the sun goes
      // down — that is what "emissive" means for a lamp at this fidelity.
      const material = p.glow > 0
        ? new THREE.MeshBasicMaterial({ color: 0xffffff })
        : new THREE.MeshLambertMaterial({ color: 0xffffff });
      if (p.double) material.side = THREE.DoubleSide;
      if (p.sway > 0) {
        addWind(material, p.sway);
        this.windMaterials.push(material);
      }
      const pool = new InstancePool(this.scene, p.geom, material, 128);
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
    for (const pool of pools) {
      const p = pool.part;
      this._p.set(x + 0.5 + offX, 0, y + 0.5 + offZ);
      this._m.compose(this._p, this._q.identity(), this._s.set(1, 1, 1));
      if (p.fixed != null) {
        this._c.setHex(p.fixed);
      } else {
        this._c.copy(color).multiplyScalar(p.tint);
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
