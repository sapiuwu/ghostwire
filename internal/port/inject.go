package port

import "ghostwire/internal/domain/input"

type InputInjectorPort interface {
	Inject(event input.InputEvent)
	ReleaseAll()
	Close()
}
