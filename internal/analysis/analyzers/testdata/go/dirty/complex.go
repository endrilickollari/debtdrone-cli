package main

import "fmt"

func ComplexFunction(a, b, c int) int {
	if a > 0 {
		if b > 0 {
			if c > 0 {
				return 1
			} else {
				return 2
			}
		} else {
			if c > 0 {
				return 3
			} else {
				return 4
			}
		}
	} else {
		for i := 0; i < 10; i++ {
			if i%2 == 0 {
				fmt.Println(i)
			} else {
				if i%3 == 0 {
					fmt.Println("fizz")
				}
			}
		}
	}
	return 0
}
