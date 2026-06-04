//go:build !windows

package main

func (a *App) UpdatePetHitTest(state PetHitTestState) error {
	return nil
}
