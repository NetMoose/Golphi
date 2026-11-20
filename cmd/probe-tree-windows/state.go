package main

import "fmt"

type editState struct {
	Text           string
	Start, End     uint32
	Focus          bool
	CaretX, CaretY int32
}

func sameState(want, got editState) error {
	if want != got {
		return fmt.Errorf("edit state mismatch: want=%+v got=%+v", want, got)
	}
	return nil
}
