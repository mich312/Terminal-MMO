# Durst World — Mechanics Mockups

> Illustrative ASCII mockups of the cozy-frontier mechanics from
> [`DESIGN_MECHANICS.md`](DESIGN_MECHANICS.md), drawn in the **glyph client's**
> idiom (the HD renderer would paint the same scene data as pixels). Glyphs and
> copy are proposals, not final art. Voice is corporate × medieval.

## Proposed new glyphs

| Glyph | Thing | Notes |
| --- | --- | --- |
| `⊓` | Workbench — *Crafting (Self-Service)* | craft station |
| `♨` | Smelter — *Ingot Synergy Furnace* | glows warm, day & night |
| `⊞` | Sawmill | timber → planks, runs offline |
| `❋` | Mill | grain → flour, runs offline |
| `▣` | Storage chest — *Cold Storage* | shared input/output buffer |
| `❒` | Sign / nameplate | names a Workspace |
| `╒` | Concession stall | player trade post |
| `▦` | Wall block (built) | placed structure |
| `⌂` | Home gate (exists) | back to the lobby |

---

## 1 · A claimed Workspace in the Wilds

Your homestead: a fenced plot you've built on open ground, machines humming,
your avatar (`S▾`) standing by the furnace. Other players can walk up and read
the sign.

```
╭─ The Wilds ───────────────────────────────── ☀ 14:20 ─╮
│ , . " , ♣ ♣ , . , . , . " , . , ♣ , . , . , . , . , . │
│ . , ╤═══════════════════════╤ . , . , ~ ~ ~ , . , . , │
│ ♣ , ║ ⊓     ♨     ⊞     ▣   ║ , . , ~ ~ ~ ~ ~ . , . , │
│ , . ║                       ║ . ♣ , ~ ~ ~ ~ ~ , . , . │
│ . , ║   ❋        S▾         ║ , . , . , ~ ~ ~ , . , . │
│ , . ║                       ║ . , . " , . , . , ♣ , . │
│ . " ║ ❒ Steurer Holdings GmbH║ , . , . , . , . , . , . │
│ , . ╘═══════════╤═══════════╝ . , ♣ , . , . , . " , . │
│ . , . " , . , . , . , . , . , . " , . , . ⌂ , . , . , │
╰────────────────────────────────────────────────────────╯
 Workspace: Steurer Holdings GmbH · 4 structures · ♨ smelting
 e interact   b build   WASD move   Tab menu   ? help
```

---

## 2 · Crafting (Self-Service)

The workbench panel. Recipes read from existing resources; `[×N]` is how many
you can make right now. Walk to a `⊓` and press `e`.

```
╭─ Crafting (Self-Service) ─────────────────────────────╮
│                                                       │
│  ▸ Wall Block        2 Cut Stone + 1 Planks     [ ×3 ]│
│    Planks            2 Timber                    [ ×8 ]│
│    Wrought Lamp      1 Gold Nugget + 1 Amber    [ ×1 ]│
│    Field Salve       1 Wild Herb + 1 Mushroom   [ ×5 ]│
│    Sawmill           6 Planks + 4 Wrought Lamp  [  —  ]│
│                                                       │
│  ─ selected ────────────────────────────────────────  │
│   ▦ Wall Block                                        │
│   "Load-bearing. Compliant with Durst facility code." │
│   needs:   ◊◊ Cut Stone ✓     ‡ Planks ✓              │
│   yields:  ▦ Wall Block ×1                            │
│                                                       │
│   in stock to make ×3                       [ e craft ]│
╰───────────────────────────────────────────────────────╯
 ↑↓ choose    e craft    q close
```

---

## 3 · A machine that runs while you're offline (the idle hero)

The whole point of "persist between visits, not during": you left, the furnace
kept smelting on the wall clock, and it greets you with what it made. Press `e`
to collect, `f` to refill the hopper.

```
╭─ Ingot Synergy Furnace ───────────────────────────────╮
│                                                       │
│   ♨  status: SMELTING               owner: steurer    │
│                                                       │
│   input    ▰▰▰▰▰▱▱▱   Gold Nugget   ×14               │
│   output   ▰▰▰▰▰▰▰▱   Gold Ingot    ×7    (cap 8)     │
│                                                       │
│   ▸ while you were away  (3h 41m):                    │
│       + 5 Gold Ingot          – 4 Gold Nugget         │
│                                                       │
│   next ingot in ~6m                  rate 1 / 20m     │
│                                                       │
│   [ e collect 7 ]            [ f refuel ]             │
╰───────────────────────────────────────────────────────╯
 e collect    f load input    q close
```

---

## 4 · Build mode (the placements layer)

Press `b` to enter build mode: a ghost of the next structure follows your
cursor, green where it fits, red where it's blocked. Placing it spends the
materials and writes one row to the `placements` table.

```
╭─ Build · placing: Sawmill ⊞ ─────────────────────────╮
│ , . " , ♣ , . , . , . " , . , . , . , ♣ , . , . , . │
│ . , . , . , . ┌─────┐ . , . , . , . , . , . , . , . │
│ , . " , . , . │ ⊞ ▒ │ . " , . , ♣ , . , . , . " , . │   ▒ ghost
│ . , ⊓ , ♨ , . └─────┘ . , . , . , . , . , . , . , . │   green ok
│ , . " , . , . , . , . , . " , . , . , . , ♣ , . , . │   red blocked
╰───────────────────────────────────────────────────────╯
 WASD move ghost   r rotate   e place (6 Planks · 4 Lamp)   q cancel
```

---

## 5 · Trade — a Durst Group Concession

A player stocks a stall; anyone can walk up and accept a posted swap. This is
the step that makes it an MMO and not single-player in a shared room.

```
╭─ Durst Group Concession — anna's stall ──────────────╮
│                                                      │
│   she offers                    she asks            │
│   ‡  Planks       ×10     ⇄     ◊◊ Cut Stone  ×6    │
│   ♨  Gold Ingot   ×2      ⇄     ◈  Geode      ×1    │
│   ⛭  Field Salve  ×5      ⇄     ψ  Grain      ×8    │
│                                                      │
│   your pack: ◊◊ Cut Stone ×11    ✓ you can do row 1 │
│                                                      │
│   [ e accept selected trade ]                        │
╰──────────────────────────────────────────────────────╯
 ↑↓ choose offer    e trade    q leave
```

---

## 6 · Kraftwerk as a real factory (chaining machines)

Kraftwerk is already a "machine hall" — promote it from set dressing to the
production floor, where the output chest of one machine feeds the next.

```
╭─ Kraftwerk — Production Floor ────────────────────────╮
│                                                       │
│   ψ grain ─▶ ❋ Mill ─▶ ▣ ─▶ ⊓ Bakery ─▶ ▣  ⊙ Bread   │
│             flour      hopper             output      │
│                                                       │
│   ‡ timber ─▶ ⊞ Sawmill ─▶ ▣ ─▶  planks for build     │
│                                                       │
│   ◆ nugget ─▶ ♨ Furnace ─▶ ▣ ─▶  ingots for trade     │
│                                                       │
│   3 lines running · next collection ~6m               │
╰───────────────────────────────────────────────────────╯
 e inspect a machine    Tab overview    q leave
```
