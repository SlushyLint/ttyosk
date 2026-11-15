//go:build !linux

package main

func watchGamepads(chan<- gamepadEvent) {}
