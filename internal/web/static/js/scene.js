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

    this.scene = new THREE.Scene();
    this.scene.background = new THREE.Color(0x0d1014);

    this.camera = new THREE.PerspectiveCamera(42, 1, 0.5, 300);
    this.zoom = DEFAULT_ZOOM;
    this.target = new THREE.Vector3(0, 0, 0);
    this.smoothed = new THREE.Vector3(0, 0, 0);
    this.yaw = 0;          // camera spin around the player, in radians
    this.targetYaw = 0;

    // Three lights, and no more: a sun for shape, a hemisphere for the sky's
    // color bouncing off the ground, and a weak fill so nothing is ever a
    // silhouette you can't read.
    this.sun = new THREE.DirectionalLight(0xffffff, 1.4);
    this.sun.position.set(-6, 12, -5);
    this.scene.add(this.sun);
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
    canvas.addEventListener('wheel', (e) => {
      e.preventDefault();
      this.zoom = clamp(this.zoom * (1 + Math.sign(e.deltaY) * 0.12), MIN_ZOOM, MAX_ZOOM);
    }, { passive: false });

    let dragging = false, lastX = 0;
    canvas.addEventListener('pointerdown', (e) => {
      if (e.button !== 2) return;
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
      // ui.Ambient's strength is how heavily the tint washes the scene: at dawn
      // it is a strong warm wash, at noon almost nothing. Read it as "how far
      // from neutral daylight are we", and drive brightness the same way.
      const s = clamp(ambient.strength ?? 0, 0, 1);
      const day = 1 - s * 0.72;
      this.sun.color.copy(tint).lerp(new THREE.Color(0xffffff), 1 - s);
      this.sun.intensity = 0.35 + day * 1.15;
      this.hemi.color.copy(tint).lerp(new THREE.Color(0xbcd8ff), 0.4);
      this.hemi.intensity = 0.2 + day * 0.55;
      this.fill.intensity = 0.12 + day * 0.2;

      const sky = tint.clone().multiplyScalar(0.35 + day * 0.4);
      this.scene.background = sky;
      this.fog.color.copy(sky);
    }
    // The edge of the sent window is the hard limit: nothing beyond it exists,
    // so the fog must close before it whatever the area's own light says. The
    // window is centered on the player, so what matters is its *half* extent —
    // and we close a little inside even that, so the last thing you see is haze
    // rather than the last row of ground.
    const edge = windowTiles > 0
      ? this.zoom + (windowTiles / 2) * 0.85
      : Infinity;

    if (light && light.r > 0) {
      // A radial light becomes fog: you can see a circle around yourself and
      // the world falls away into the dark beyond it. The camera sits back from
      // the player, so the distances are offset by the camera's own distance.
      this.fog.far = Math.min(this.zoom + light.r * 1.05, edge);
      this.fog.near = Math.min(this.zoom + light.r * 0.35, this.fog.far - 4);
      if (light.sunless) {
        // Underground: no sky at all, so the fog goes to black rather than to
        // whatever color the sky above happens to be.
        this.fog.color.setHex(0x04060a);
        this.scene.background = new THREE.Color(0x04060a);
      }
    } else {
      this.fog.far = Math.min(this.zoom + 60, edge);
      this.fog.near = Math.max(this.zoom * 0.5, this.fog.far - 26);
    }
  }

  /** follow points the camera at a world position. */
  follow(x, z, instant = false) {
    this.target.set(x, 0, z);
    if (instant) this.smoothed.copy(this.target);
  }

  update(dt) {
    // Lag the camera behind the player: a little inertia reads as weight, and
    // it hides the fact that movement is discrete tile steps underneath.
    this.smoothed.lerp(this.target, Math.min(1, dt * 7));
    this.yaw += (this.targetYaw - this.yaw) * Math.min(1, dt * 8);

    const horiz = Math.cos(PITCH) * this.zoom;
    const vert = Math.sin(PITCH) * this.zoom;
    this.camera.position.set(
      this.smoothed.x + Math.sin(this.yaw) * horiz,
      vert,
      this.smoothed.z + Math.cos(this.yaw) * horiz,
    );
    this.camera.lookAt(this.smoothed);
  }

  render() {
    this.renderer.render(this.scene, this.camera);
  }
}

function clamp(v, lo, hi) { return v < lo ? lo : v > hi ? hi : v; }
