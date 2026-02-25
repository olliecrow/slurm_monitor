package uifmt

import (
	"fmt"
)

func Ratio(alloc, total int) string {
	return fmt.Sprintf("%d/%d", alloc, total)
}

func Percent(v float64, ok bool) string {
	if !ok {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", v)
}

func MemMB(v int) string {
	if v >= 1024*1024 {
		return fmt.Sprintf("%.1fT", float64(v)/1024.0/1024.0)
	}
	if v >= 1024 {
		return fmt.Sprintf("%.1fG", float64(v)/1024.0)
	}
	return fmt.Sprintf("%dM", v)
}

func MemPair(allocMB, totalMB int) string {
	return fmt.Sprintf("%s/%s", MemMB(allocMB), MemMB(totalMB))
}
