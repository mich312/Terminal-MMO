/* The scene: camera, light and sky.
 *
 * The camera is a tilted 3/4 follow-cam — high enough to read the layout of a
 * village, low enough that a cathedral has a face and not just a roof. It
 * tracks the local player with a lag, so walking feels like the world moving
 * under you rather than the camera being welded to your head.
 *
 * Lighting is where the terminal clients' work is reused most directly. The
 * server already computes an ambient sky tint and strength for the time of day
 * (ui.Ambient) and a radial light for areas that have one (the Wilds' discovery
 * circle, a cave's lantern). Here those become a real sun, a real ambient term,
 * and distance fog — so fog-of-war stops being dimmed tiles and becomes actual
 * darkness you walk a hole in.
 */

import * as THREE from 'three';

const PITCH = 52 * Math.PI / 180; // camera tilt from horizontal
const MIN_ZOOM = 9, MAX_ZOOM = 34, DEFAULT_ZOOM = 17;

/* The action camera (docs/SWORDPLAY_PLAN.md): an over-the-shoulder chase-cam
   with the hero framing the left third of the screen. The character faces
   where they move, not where the camera looks — you watch them fight. */
const ACT_MIN_DIST = 3, ACT_MAX_DIST = 9, ACT_DIST = 5.2;
const ACT_EYE = 1.35;       // look-at height: about the hero's collarbone
const ACT_SHOULDER = 0.55;  // sideways offset — the hero out of the crosshair
const ACT_FOG_ANCHOR = 6;   // stands in for "camera distance" in fog math

/* The two ends of the warm/cool axis the lighting is built on, plus the color
   the ground bounces back up into everything standing on it. */
const WARM = new THREE.Color(0xffd9a8);
const SKY_BLUE = new THREE.Color(0x9dc4f0);
const GROUND_BOUNCE = new THREE.Color(0x3a3226);
const MOON = new THREE.Color(0xaec4e8);
const HAZE = new THREE.Color(0xb9d4ea); // pale sky just above the skyline

/** SkyDome is a gradient sky: an inverted sphere shaded from a zenith color
 *  through a horizon color to a ground haze. A flat background color is the
 *  single most obvious "this is a 3D toy" tell, and a gradient costs two
 *  triangles' worth of thought.
 *
 *  It doubles as the scene's environment: PMREM-filtered, it becomes the
 *  ambient light that PBR materials reflect, so the sky at dusk actually tints
 *  the things standing under it. */
class SkyDome {
  constructor() {
    this.uniforms = {
      uTop: { value: new THREE.Color(0x2b5a8c) },
      uHorizon: { value: new THREE.Color(0xa8c4d8) },
      uBottom: { value: new THREE.Color(0x1a1d22) },
    };
    const material = new THREE.ShaderMaterial({
      uniforms: this.uniforms,
      side: THREE.BackSide,
      depthWrite: false,
      fog: false,
      vertexShader: `
        varying vec3 vDir;
        void main() {
          vDir = normalize(position);
          gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
        }`,
      // A raw ShaderMaterial doesn't get three's tone mapping and color space
      // for free, so pull in the same chunks every other material uses —
      // otherwise the sky is the one thing in frame that isn't graded.
      fragmentShader: `
        uniform vec3 uTop;
        uniform vec3 uHorizon;
        uniform vec3 uBottom;
        varying vec3 vDir;
        void main() {
          float h = normalize(vDir).y;
          vec3 col = mix(uHorizon, uTop, smoothstep(0.0, 0.55, h));
          col = mix(uBottom, col, smoothstep(-0.3, 0.015, h));
          gl_FragColor = vec4(col, 1.0);
          #include <tonemapping_fragment>
          #include <colorspace_fragment>
        }`,
    });
    this.material = material;
    this.mesh = new THREE.Mesh(new THREE.SphereGeometry(240, 24, 16), material);
    this.mesh.renderOrder = -1;
    this.mesh.frustumCulled = false;
    // A separate one-mesh scene to bake the environment from, so baking never
    // has to reach into (or disturb) the live scene.
    this.envScene = new THREE.Scene();
    this.envScene.add(new THREE.Mesh(new THREE.SphereGeometry(10, 16, 12), material));
  }

  set(top, horizon, bottom) {
    this.uniforms.uTop.value.copy(top);
    this.uniforms.uHorizon.value.copy(horizon);
    this.uniforms.uBottom.value.copy(bottom);
  }
}

export class WorldScene {
  constructor(canvas) {
    this.canvas = canvas;
    this.renderer = new THREE.WebGLRenderer({
      canvas,
      antialias: true,
      powerPreference: 'high-performance',
    });
    this.renderer.setPixelRatio(Math.min(devicePixelRatio || 1, 2));
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    // Filmic tone mapping. Without it, a bright sun clips flat white and a lit
    // window blows out; ACES rolls the highlights off and keeps saturation in
    // the shoulder, which is most of the difference between "3D shapes" and "a
    // rendered scene".
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    this.renderer.toneMappingExposure = 1.0;
    // Shadows are what put objects *on* the ground rather than hovering over
    // it. Soft PCF is the cheapest kind that doesn't look like a stencil.
    this.renderer.shadowMap.enabled = true;
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;

    this.scene = new THREE.Scene();
    this.scene.background = new THREE.Color(0x0d1014);
    this.sky = new SkyDome();
    this.scene.add(this.sky.mesh);

    this.camera = new THREE.PerspectiveCamera(42, 1, 0.5, 300);
    this.zoom = DEFAULT_ZOOM;
    this.target = new THREE.Vector3(0, 0, 0);
    this.smoothed = new THREE.Vector3(0, 0, 0);
    this.yaw = 0;          // camera spin around the player, in radians
    this.targetYaw = 0;

    // Action mode state. camYaw/camPitch are mouse-look; lockPoint, when set,
    // steers the yaw so the locked target stays in frame.
    this.mode = 'top';
    this.camYaw = 0;
    this.camPitch = 0.16;
    this.actDist = ACT_DIST;
    this.lockPoint = null;
    this.onPointerLockLost = null;

    // Three lights, and no more: a sun for shape, a hemisphere for the sky's
    // color bouncing off the ground, and a weak fill so nothing is ever a
    // silhouette you can't read.
    this.sun = new THREE.DirectionalLight(0xffffff, 1.4);
    this.sun.position.set(-6, 12, -5);
    this.sun.castShadow = true;
    // The shadow camera is small and follows the player (see update). A tight
    // ortho box around what's actually on screen buys far more resolution than
    // a big map ever would — a 2048² map over 44 tiles is ~46 texels per tile.
    this.sun.shadow.mapSize.set(2048, 2048);
    this.sun.shadow.camera.near = 1;
    this.sun.shadow.camera.far = 120;
    // normalBias, not bias: it offsets along the surface normal, which kills
    // acne on the big flat ground planes without detaching the contact shadows
    // that make props feel planted.
    this.sun.shadow.normalBias = 0.035;
    this.sun.shadow.bias = -0.0006;
    this.scene.add(this.sun);
    this.scene.add(this.sun.target);
    this.hemi = new THREE.HemisphereLight(0xbcd8ff, 0x2b2a26, 0.6);
    this.scene.add(this.hemi);
    this.fill = new THREE.AmbientLight(0xffffff, 0.25);
    this.scene.add(this.fill);

    this.fog = new THREE.Fog(0x0d1014, 40, 90);
    this.scene.fog = this.fog;

    this.resize();
    window.addEventListener('resize', () => this.resize());

    // Zoom with the wheel; drag with the right mouse button to spin the camera
    // around your character (some things hide behind a tall building otherwise).
    // In action mode the wheel adjusts the chase distance instead, and the
    // right button belongs to the guard.
    canvas.addEventListener('wheel', (e) => {
      e.preventDefault();
      if (this.mode === 'action') {
        this.actDist = clamp(this.actDist * (1 + Math.sign(e.deltaY) * 0.1), ACT_MIN_DIST, ACT_MAX_DIST);
      } else {
        this.zoom = clamp(this.zoom * (1 + Math.sign(e.deltaY) * 0.12), MIN_ZOOM, MAX_ZOOM);
      }
    }, { passive: false });

    let dragging = false, lastX = 0;
    canvas.addEventListener('pointerdown', (e) => {
      if (e.button !== 2 || this.mode === 'action') return;
      dragging = true; lastX = e.clientX;
      canvas.setPointerCapture(e.pointerId);
    });
    canvas.addEventListener('pointermove', (e) => {
      if (!dragging) return;
      this.targetYaw += (e.clientX - lastX) * 0.006;
      lastX = e.clientX;
    });
    const stop = () => { dragging = false; };
    canvas.addEventListener('pointerup', stop);
    canvas.addEventListener('pointercancel', stop);
    canvas.addEventListener('contextmenu', (e) => e.preventDefault());

    // Mouse-look, live only while the pointer is locked to the canvas.
    document.addEventListener('mousemove', (e) => {
      if (this.mode !== 'action' || document.pointerLockElement !== canvas) return;
      this.camYaw -= e.movementX * 0.0032;
      this.camPitch = clamp(this.camPitch + e.movementY * 0.0026, -0.3, 0.95);
    });
    document.addEventListener('pointerlockchange', () => {
      if (this.mode === 'action' && document.pointerLockElement !== canvas) {
        // Esc released the pointer. The mode stays; the next click on the
        // world re-engages mouse-look (see input.js).
        this.onPointerLockLost?.();
      }
    });
  }

  /** setMode switches between the top-down follow-cam and the action camera.
   *  Entering action mode locks the pointer (we're called from a key press, a
   *  user gesture, so the browser allows it); leaving releases it. */
  setMode(mode) {
    if (this.mode === mode) return;
    this.mode = mode;
    if (mode === 'action') {
      // Start looking the way the world does: over the character's shoulder.
      this.camYaw = Math.PI; // facing north — the classic establishing angle
      this.camPitch = 0.16;
      this.relock();
    } else {
      this.lockPoint = null;
      if (document.pointerLockElement === this.canvas) document.exitPointerLock?.();
    }
  }

  /** relock (re)acquires the pointer for mouse-look. The browser may refuse
   *  outside a user gesture — callers treat that as "wait for the next click",
   *  so the rejection is expected and swallowed. */
  relock() {
    if (this.mode !== 'action') return;
    const p = this.canvas.requestPointerLock?.();
    p?.catch?.(() => {});
  }

  /** forward is the action camera's ground-plane look direction. */
  forward() {
    return { x: Math.sin(this.camYaw), z: Math.cos(this.camYaw) };
  }

  resize() {
    const w = this.canvas.clientWidth || window.innerWidth;
    const h = this.canvas.clientHeight || window.innerHeight;
    this.renderer.setSize(w, h, false);
    this.camera.aspect = w / Math.max(1, h);
    this.camera.updateProjectionMatrix();
  }

  /** tilesInView estimates how many tiles the camera can see, so the client can
   *  ask the server for a window that matches the screen — no more, no less. */
  tilesInView() {
    if (this.mode === 'action') {
      // The chase-cam looks outward, not down: how much ground is visible is
      // set by the fog, not the zoom. Ask for a window that fills out to the
      // fog line in every direction (the camera can spin freely).
      return { w: 44, h: 36 };
    }
    const h = 2 * this.zoom * Math.tan((this.camera.fov * Math.PI / 180) / 2);
    const w = h * this.camera.aspect;
    // Generous margins, on purpose. The window's edge is a cliff into empty
    // space, and perspective makes the far side of the view much wider than
    // this flat estimate — so we ask for noticeably more ground than we expect
    // to see, and let fog handle the rest. Under-asking is visible; over-asking
    // costs a few hundred instanced tiles.
    return {
      w: Math.ceil(w * 1.6) + 10,
      h: Math.ceil(h / Math.sin(PITCH) * 1.3) + 10,
    };
  }

  /** applyLighting turns the server's sky and radial light into the scene's.
   *  windowTiles is how much ground the server is sending, so the fog can be
   *  pulled in to hide the window's edge — the world should fade into haze, not
   *  end at a cliff. */
  applyLighting(ambient, light, windowTiles) {
    if (ambient) {
      const tint = new THREE.Color(ambient.hex || '#ffffff');
      // `night` is 0 at midday and 1 at the darkest hour, normalized by the
      // server against its own day cycle. Everything below is a lerp along it.
      const n = clamp(ambient.night ?? 0, 0, 1);

      // Warm key, cool fill. Real daylight is a warm sun against a blue sky
      // bounce, and separating the two gives a surface a lit side and a shaded
      // side that differ in *hue* rather than only in brightness — which reads
      // as depth far more than raw intensity does.
      //
      // After dark the key doesn't just dim, it changes character: a faint,
      // cool moon standing in for the sun, with the sky bounce taking over as
      // the main source. A night that is only a dimmer day looks like a bug.
      this.sun.color.copy(tint).lerp(WARM, 0.5 * (1 - n) + 0.15).lerp(MOON, n * 0.8);
      this.sun.intensity = mix(2.35, 0.16, n);
      this.hemi.color.copy(SKY_BLUE).lerp(tint, 0.35 + 0.4 * n);
      this.hemi.groundColor.copy(GROUND_BOUNCE).lerp(tint, 0.3 * n);
      this.hemi.intensity = mix(0.55, 0.3, n);
      this.fill.intensity = mix(0.14, 0.08, n);

      // Sky color is mostly a question about the *horizon*, not the zenith: the
      // camera looks down at 52°, so the only sky ever on screen is the band
      // just above the skyline. Tinting the top of the dome has almost no
      // visible effect — a lesson learned by making a very blue zenith nobody
      // could see, under a white horizon everybody could.
      //
      // `neutral` is how uncolored ui.Ambient's tint currently is. At midday
      // it's essentially white, so we impose our own blue; at dawn and dusk
      // it's a strong amber that should lead instead.
      const neutral = 1 - clamp((ambient.strength ?? 0) / 0.25, 0, 1);
      const horizon = tint.clone().lerp(HAZE, 0.6 * neutral).multiplyScalar(mix(0.92, 0.16, n));
      const top = tint.clone().lerp(SKY_BLUE, 0.35 + 0.55 * neutral).multiplyScalar(mix(0.6, 0.06, n));
      const bottom = tint.clone().multiplyScalar(mix(0.3, 0.04, n));
      this.sky.set(top, horizon, bottom);
      this.fog.color.copy(horizon);
      this.refreshEnvironment(horizon);
    }
    // The edge of the sent window is the hard limit: nothing beyond it exists,
    // so the fog must close before it whatever the area's own light says. The
    // window is centered on the player, so what matters is its *half* extent —
    // and we close a little inside even that, so the last thing you see is haze
    // rather than the last row of ground. All fog distances are measured from
    // the camera, which in action mode sits close over the player's shoulder —
    // a fixed anchor stands in for the top-down zoom there.
    const anchor = this.mode === 'action' ? ACT_FOG_ANCHOR : this.zoom;
    const edge = windowTiles > 0
      ? anchor + (windowTiles / 2) * 0.85
      : Infinity;

    if (light && light.r > 0) {
      // A radial light becomes fog: you can see a circle around yourself and
      // the world falls away into the dark beyond it. The camera sits back from
      // the player, so the distances are offset by the camera's own distance.
      this.fog.far = Math.min(anchor + light.r * 1.05, edge);
      this.fog.near = Math.min(anchor + light.r * 0.35, this.fog.far - 4);
      if (light.sunless) {
        // Underground: no sky at all, so the fog goes to black rather than to
        // whatever color the sky above happens to be.
        this.fog.color.setHex(0x04060a);
        this.scene.background = new THREE.Color(0x04060a);
      }
    } else {
      this.fog.far = Math.min(anchor + 60, edge);
      this.fog.near = Math.max(anchor * 0.5, this.fog.far - 26);
    }
  }

  /** refreshEnvironment re-bakes the sky into the image-based light that PBR
   *  materials reflect. The bake costs a few milliseconds, and the sky changes
   *  over minutes, so it is thresholded: only a visible shift in the horizon
   *  color pays for a new one. */
  refreshEnvironment(horizon) {
    if (this._envColor && colorDelta(this._envColor, horizon) < 0.02) return;
    this._envColor = horizon.clone();
    if (!this._pmrem) this._pmrem = new THREE.PMREMGenerator(this.renderer);
    const baked = this._pmrem.fromScene(this.sky.envScene, 0.02);
    this.scene.environment?.dispose?.();
    this.scene.environment = baked.texture;
  }

  /** follow points the camera at a world position. */
  follow(x, z, instant = false) {
    this.target.set(x, 0, z);
    if (instant) this.smoothed.copy(this.target);
  }

  update(dt) {
    // Lag the camera behind the player: a little inertia reads as weight, and
    // it hides the fact that movement is discrete tile steps underneath. The
    // chase-cam tracks tighter — at melee range, lag reads as seasickness.
    this.smoothed.lerp(this.target, Math.min(1, dt * (this.mode === 'action' ? 11 : 7)));
    this.yaw += (this.targetYaw - this.yaw) * Math.min(1, dt * 8);

    if (this.mode === 'action') {
      this.updateAction(dt);
    } else {
      const horiz = Math.cos(PITCH) * this.zoom;
      const vert = Math.sin(PITCH) * this.zoom;
      this.camera.position.set(
        this.smoothed.x + Math.sin(this.yaw) * horiz,
        vert,
        this.smoothed.z + Math.cos(this.yaw) * horiz,
      );
      this.camera.lookAt(this.smoothed);
    }

    // The sky is infinitely far away, so it rides with the camera. Without
    // this it would sit at the world origin, and walking a few hundred tiles
    // out into the Wilds would leave it behind.
    this.sky.mesh.position.copy(this.camera.position);

    // Walk the shadow camera along with the player and size its box to what is
    // actually on screen. Shadow resolution is the box divided by the map, so a
    // box that tracks the view is worth more than a much larger shadow map.
    // The chase-cam sees less ground, so its box is tighter — crisper shadows
    // on the two fighters that fill the frame.
    const span = (this.mode === 'action' ? 16 : this.zoom) * 1.15;
    const sc = this.sun.shadow.camera;
    if (sc.right !== span) {
      sc.left = -span; sc.right = span;
      sc.top = span; sc.bottom = -span;
      sc.updateProjectionMatrix();
    }
    // Keep the sun's direction fixed in world space (it's a sun, not a lamp) by
    // moving light and target together.
    this.sun.target.position.copy(this.smoothed);
    this.sun.position.set(
      this.smoothed.x - 14, 26, this.smoothed.z - 11,
    );
  }

  /** updateAction is the over-the-shoulder camera: orbit the mouse-look yaw
   *  and pitch around the hero, offset to the right so they frame the left
   *  third, and — locked on — steer the yaw so both fighters stay in shot. */
  updateAction(dt) {
    if (this.lockPoint) {
      const dx = this.lockPoint.x - this.smoothed.x;
      const dz = this.lockPoint.z - this.smoothed.z;
      if (dx * dx + dz * dz > 0.05) {
        const want = Math.atan2(dx, dz);
        let d = want - this.camYaw;
        while (d > Math.PI) d -= Math.PI * 2;
        while (d < -Math.PI) d += Math.PI * 2;
        this.camYaw += d * Math.min(1, dt * 6);
      }
    }

    const fx = Math.sin(this.camYaw), fz = Math.cos(this.camYaw);
    const rx = -fz, rz = fx; // the camera's right hand on the ground plane
    const horiz = this.actDist * Math.cos(this.camPitch);
    const lift = this.actDist * Math.sin(this.camPitch);
    this.camera.position.set(
      this.smoothed.x - fx * horiz + rx * ACT_SHOULDER,
      ACT_EYE + lift,
      this.smoothed.z - fz * horiz + rz * ACT_SHOULDER,
    );
    this.camera.lookAt(
      this.smoothed.x + fx * 3 + rx * ACT_SHOULDER,
      ACT_EYE - this.camPitch * 1.6,
      this.smoothed.z + fz * 3 + rz * ACT_SHOULDER,
    );
  }

  render() {
    this.renderer.render(this.scene, this.camera);
  }
}

function clamp(v, lo, hi) { return v < lo ? lo : v > hi ? hi : v; }
function mix(a, b, t) { return a + (b - a) * t; }

/** colorDelta is how far apart two colors are. THREE.Color has no distanceTo —
 *  that's Vector3 — and this is only ever used as a "has the sky moved enough
 *  to be worth re-baking" threshold, so component distance is plenty. */
function colorDelta(a, b) {
  const dr = a.r - b.r, dg = a.g - b.g, db = a.b - b.b;
  return Math.sqrt(dr * dr + dg * dg + db * db);
}
