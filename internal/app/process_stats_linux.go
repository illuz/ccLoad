package app

import "os"

func readCurrentRSSBytes() uint64 {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	return parseStatmResidentBytes(string(data), os.Getpagesize())
}
