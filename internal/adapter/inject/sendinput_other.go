//go:build !windows

package inject

import (
	"ghostwire/internal/domain"
	"ghostwire/internal/domain/input"
)

type SendInputInjector struct{}

func NewSendInputInjector() *SendInputInjector {
	return &SendInputInjector{}
}

func (inj *SendInputInjector) Inject(_ input.InputEvent) {
	panic(domain.NewError(domain.ErrUnsupported, "input injection requires Windows"))
}

func (inj *SendInputInjector) ReleaseAll() {}

func (inj *SendInputInjector) Close() {}
