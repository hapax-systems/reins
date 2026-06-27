package grammar

import (
	"math"
	"strings"
)

var sparkGlyphs = []rune(" ▁▂▃▄▅▆▇█")

func resample(series []float64, n int) []float64 {
	if n <= 0 || len(series) == 0 {
		return nil
	}
	if len(series) <= n {
		out := make([]float64, len(series))
		copy(out, series)
		return out
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		start := i * len(series) / n
		end := (i + 1) * len(series) / n
		if end <= start {
			end = start + 1
		}
		if end > len(series) {
			end = len(series)
		}
		sum, count := 0.0, 0
		for j := start; j < end; j++ {
			sum += series[j]
			count++
		}
		if count > 0 {
			out[i] = sum / float64(count)
		}
	}
	return out
}

func clampNonNeg(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}

func seriesMax(series []float64) float64 {
	max := 0.0
	for _, v := range series {
		v = clampNonNeg(v)
		if v > max {
			max = v
		}
	}
	return max
}

// Sparkline renders the series as a width-N eighths-block sparkline.
func Sparkline(series []float64, width int) string {
	if width <= 0 || len(series) == 0 {
		return ""
	}
	pts := resample(series, width)
	max := seriesMax(pts)
	buf := make([]rune, 0, len(pts))
	for _, v := range pts {
		v = clampNonNeg(v)
		var level int
		if max <= 0 {
			if v == 0 {
				level = 0
			} else {
				level = 1
			}
		} else {
			level = int(v / max * 8)
			if level < 0 {
				level = 0
			} else if level > 8 {
				level = 8
			}
		}
		buf = append(buf, sparkGlyphs[level])
	}
	return string(buf)
}

// BrailleSparkline renders the series as a width-N braille (2x4 dot) sparkline.
func BrailleSparkline(series []float64, width int) string {
	if width <= 0 || len(series) == 0 {
		return ""
	}
	pts := resample(series, 2*width)
	max := seriesMax(pts)
	leftDots := []int{0x40, 0x04, 0x02, 0x01}
	rightDots := []int{0x80, 0x20, 0x10, 0x08}
	buf := make([]rune, 0, width)
	for i := 0; i < width; i++ {
		code := 0x2800
		for sub := 0; sub < 2; sub++ {
			idx := 2*i + sub
			if idx >= len(pts) {
				break
			}
			v := clampNonNeg(pts[idx])
			var level int
			if max <= 0 {
				if v == 0 {
					level = 0
				} else {
					level = 1
				}
			} else {
				level = int(v / max * 4)
				if level < 0 {
					level = 0
				} else if level > 4 {
					level = 4
				}
			}
			dots := leftDots
			if sub == 1 {
				dots = rightDots
			}
			for k := 0; k < level; k++ {
				code |= dots[k]
			}
		}
		buf = append(buf, rune(code))
	}
	return string(buf)
}

// NetSparkline renders a signed net-flow eighths sparkline across width buckets.
func NetSparkline(net []float64, width int) string {
	if width <= 0 || len(net) == 0 {
		return ""
	}
	pts := resample(net, width)
	max := 0.0
	for _, v := range pts {
		if a := math.Abs(v); a > max {
			max = a
		}
	}
	var b strings.Builder
	for _, v := range pts {
		if v == 0 {
			b.WriteString("·")
			continue
		}
		level := int(math.Abs(v) / max * 8)
		if level < 1 {
			level = 1
		} else if level > 8 {
			level = 8
		}
		glyph := string(sparkGlyphs[level])
		if v < 0 {
			glyph = C("mut", glyph)
		}
		b.WriteString(glyph)
	}
	return b.String()
}
