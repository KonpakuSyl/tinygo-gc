package main

import "regions"

func leaf() map[int]int {
	m := make(map[int]int)
	for i := 0; i < 128; i++ {
		m[i] = i
	}
	return m
}

func main() {
	r := regions.New()
	var m map[int]int
	regions.Do(r, func() {
		m = leaf()
		m[128] = 128
	})
	if m[128] != 128 {
		panic("regions map failed")
	}
	r.Release()
}
