//go:build !linux && !darwin

package app

func readProcessRusage() (userSeconds, systemSeconds float64, maxRSSBytes uint64, ok bool) {
	return 0, 0, 0, false
}

func readCurrentRSSBytes() uint64 { return 0 }
