package game

import "github.com/durst-group/durstworld/internal/ui"

// Avatar editing shared by the glyph character panel (root.go) and the HD one
// (hd_ui.go), so both clients customize the same way and stay in sync.

// CharFields is the number of editable character fields (style, color, hat).
const CharFields = 3

// OwnedHats returns the accessory indices the player may wear: none (0) plus
// every hat they've unlocked, in order.
func OwnedHats(ctx *Ctx) []int {
	list := []int{0}
	for i := 1; i < NumAccessories(); i++ {
		if ctx.Hats[i] {
			list = append(list, i)
		}
	}
	return list
}

// CycleAvatarField changes one character field (0 style, 1 color, 2 hat) by d
// and persists the result. Hat cycling is limited to unlocked hats.
func CycleAvatarField(ctx *Ctx, field, d int) {
	cur, ok := ctx.World.Self(ctx.Name)
	if !ok {
		return
	}
	switch field {
	case 0:
		ctx.World.SetAvatar(ctx.Name, wrapIdx(cur.Style+d, NumAvatarStyles()), cur.Accessory)
	case 1:
		idx := wrapIdx(ui.AvatarColorIndex(cur.Color)+d, ui.NumAvatarColors())
		ctx.World.SetColor(ctx.Name, ui.AvatarColorByIndex(idx))
	case 2:
		ctx.World.SetAvatar(ctx.Name, cur.Style, cycleOwnedHat(ctx, cur.Accessory, d))
	}
	if p, ok := ctx.World.Self(ctx.Name); ok {
		ctx.Store.SaveAvatar(ctx.Name, string(p.Color), p.Style, p.Accessory)
	}
}

func cycleOwnedHat(ctx *Ctx, cur, d int) int {
	list := OwnedHats(ctx)
	pos := 0
	for i, v := range list {
		if v == cur {
			pos = i
		}
	}
	pos = ((pos+d)%len(list) + len(list)) % len(list)
	return list[pos]
}
