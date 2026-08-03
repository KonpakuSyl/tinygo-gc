package main

import (
	"regions"
	"time"
)

func printValue(v int) {
	m := make(map[string]int)
	m["value"] = v
	time.Sleep(time.Nanosecond)
	println(m["value"])
}

func useManualMap(r *regions.Region, m map[int]int) {
	println(m[1])
}

func startManualMapTask() {
	r := regions.New()
	var m map[int]int
	regions.Do(r, func() {
		m = make(map[int]int)
		m[1] = 1
	})
	go useManualMap(r, m)
}

func main() {
	go printValue(7)
	startManualMapTask()
}
