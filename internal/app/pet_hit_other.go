//go:build !windows

package app

func (a *App) UpdatePetHitTest(state PetHitTestState) error {
	_ = state
	return nil
}
