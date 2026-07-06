package worldgen

// The seasonal snowline: in real winter, snow cover creeps down from the
// heights into the cool lowlands, deepest in mid-January, gone by summer — so
// the infinite map has a calendar, not just a clock.
//
// The cover is strictly cosmetic: it flips a cell's Color (and the Snowy flag
// the Wilds' CellTile reads to swap the HD ground to snow) and freezes its
// shimmer, but never touches Biome, Glyph, Walkable or anything structural —
// settlements, items, claims and collision are computed from the same
// summer-stable fields year round. Only natural terrain dusts; village
// grounds, roads and worksites keep their look (and their glyph+color keyed
// harvest rolls) in every season.

import (
	"math"
	"sync"
	"time"
)

// Clock is the calendar the seasonal cover reads. It defaults to the wall
// clock; tests and art tools pin it to sample a fixed date (ui.Now's twin —
// worldgen stays a leaf package, so it keeps its own).
var Clock = time.Now

// SeasonSnow is how deep winter is on t's date: 1 in mid-January, 0 in
// mid-July, easing through the equinoxes — the knob the snowline shifts by.
func SeasonSnow(t time.Time) float64 {
	doy := float64(t.YearDay())
	return 0.5 + 0.5*math.Cos(2*math.Pi*(doy-15)/365.25)
}

// seasonCache memoizes the day's snow depth so At doesn't recompute the
// calendar for every cell of every frame. Quantized to the day, which also
// guarantees every cell sampled on one day agrees.
var seasonCache struct {
	sync.Mutex
	key  int
	snow float64
}

func seasonSnowNow() float64 {
	t := Clock()
	key := t.Year()*1000 + t.YearDay()
	seasonCache.Lock()
	defer seasonCache.Unlock()
	if seasonCache.key != key {
		seasonCache.key, seasonCache.snow = key, SeasonSnow(t)
	}
	return seasonCache.snow
}

// The summer snowline (the ladder's own thresholds) and how far a full winter
// pushes each: at season 1 the temp line reaches 0.56 and the region gate
// 0.55, so snow settles well below the highlands across the cool half of the
// map — a season you notice, not a sprinkle.
const (
	snowTempLine   = 0.40
	snowRegionLine = 0.42
	seasonTempAmp  = 0.16
	seasonRegAmp   = 0.13
)

// snowCoverAt reports whether winter currently dusts a natural cell with this
// climate. Cells that are Snow biome year-round never reach here.
func snowCoverAt(temp, region float64) bool {
	s := seasonSnowNow()
	if s < 0.05 {
		return false // high summer: bare
	}
	return temp < snowTempLine+seasonTempAmp*s && region < snowRegionLine+seasonRegAmp*s
}

// winterized applies the season's cover to a natural terrain cell: ground
// tones go to the snow palette, any shimmer freezes still, and Snowy tells the
// renderers to lay white ground. Props (trees, flowers, bushes) keep their own
// colors — green firs over white ground read as winter woods.
func winterized(c Cell, temp, region float64) Cell {
	if !snowCoverAt(temp, region) {
		return c
	}
	c.Snowy = true
	switch c.Glyph {
	case '·':
		c.Color = "#E8EEF5" // open snow, the Snow biome's own ground tone
	case ',':
		c.Color = "#D4DEEA" // drifted tuft
	case '°':
		c.Color = "#C2CCD6" // iced rock
	case '▲':
		c.Color = "#EAF0F7" // a bare peak takes a winter cap
	}
	c.AnimA, c.AnimB, c.Frames = "", "", nil // a frozen marsh stops flowing
	return c
}
